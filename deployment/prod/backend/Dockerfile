FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 app
ENV TZ=Asia/Shanghai GIN_MODE=release
WORKDIR /app
COPY server seed /app/
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
CMD ["-f", "/app/settings.yaml"]
