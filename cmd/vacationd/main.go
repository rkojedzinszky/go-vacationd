package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/emersion/go-smtp"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/namsral/flag"

	"github.com/rkojedzinszky/go-vacationd/pkg/backend"
	"github.com/rkojedzinszky/go-vacationd/pkg/interfaces"
	"github.com/rkojedzinszky/go-vacationd/pkg/postfixadminvacation"
	"github.com/rkojedzinszky/go-vacationd/pkg/ratelimiter"
)

var (
	autoreplyDomain   = flag.String("autoreply-domain", "autoreply", "Domain used to receive mails subject to auto-reply")
	lmtpListenAddress = flag.String("lmtp-listen-address", ":1025", "LMTP listen address")
	smtpServerAddress = flag.String("smtp-server-address", "", "SMTP Address used to send replies")
	smtpServerPort    = flag.Int("smtp-server-port", 25, "SMTP Port used to send replies")

	postfixAdminDSN = flag.String("postfixadmin-dsn", "", "Database DSN to use with postfixadmin replygenerator")

	errDBDriverNotSupported = errors.New("db driver not supported")

	lmtpDebug = flag.Bool("lmtp-debug", false, "Debug LMTP connection")
)

func main() {
	flag.Parse()

	if *smtpServerAddress == "" {
		log.Fatal("smtp-server-address needs to be specified")
	}

	lis, err := net.Listen("tcp", *lmtpListenAddress)
	if err != nil {
		log.Fatal(err)
	}
	defer lis.Close()

	// send reminders once a day by default
	rl := ratelimiter.New(24 * time.Hour)

	// create replygenerator
	var rg interfaces.ReplyGenerator
	if *postfixAdminDSN != "" {
		rg, err = newPostfixadminReplyGenerator(*postfixAdminDSN)
		if err != nil {
			log.Fatal(err)
		}
	}

	// create backend
	backend, err := backend.New(*autoreplyDomain, *smtpServerAddress, *smtpServerPort, rl, rg)
	if err != nil {
		log.Fatal(err)
	}

	// create lmtpd server
	s := smtp.NewServer(backend)
	s.LMTP = true
	s.AuthDisabled = true
	s.ReadTimeout = time.Minute
	s.Domain = *autoreplyDomain

	if *lmtpDebug {
		s.Debug = os.Stdout
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg := &sync.WaitGroup{}

	// run ratelimiter
	wg.Add(1)
	go func() {
		defer wg.Done()

		rl.Run(ctx)
	}()

	// run lmtp daemon
	wg.Add(1)
	go func() {
		defer wg.Done()

		go s.Serve(lis)

		<-ctx.Done()

		ctxto, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		s.Shutdown(ctxto)
	}()

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT)
	<-sigchan

	log.Print("Exiting...")
	cancel()
	wg.Wait()
}

func newPostfixadminReplyGenerator(dsn string) (rg interfaces.ReplyGenerator, err error) {
	var db *sql.DB

	if strings.HasPrefix(dsn, "postgres://") {
		db, err = sql.Open("pgx", dsn)
	} else if strings.HasPrefix(dsn, "mysql://") {
		db, err = sql.Open("mysql", strings.TrimPrefix(dsn, "mysql://"))
	} else {
		err = errDBDriverNotSupported
	}

	if err != nil {
		return
	}

	rg, err = postfixadminvacation.New(db)

	return
}
