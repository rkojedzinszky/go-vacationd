FROM scratch

ARG TARGETARCH

COPY go-vacationd.${TARGETARCH} /go-vacationd

USER 65534

CMD ["/go-vacationd"]
