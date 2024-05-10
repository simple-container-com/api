FROM gcr.io/distroless/base-debian12

WORKDIR /

ADD dist/cloud-helpers /cloud-helpers

EXPOSE 8080

ENTRYPOINT ["/cloud-helpers"]