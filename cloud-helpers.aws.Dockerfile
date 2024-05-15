#FROM gcr.io/distroless/base-debian12
FROM public.ecr.aws/lambda/provided:al2023

WORKDIR /

ADD dist/cloud-helpers /cloud-helpers

EXPOSE 8080

ENTRYPOINT ["/cloud-helpers"]