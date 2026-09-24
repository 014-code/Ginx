FROM node:24-alpine AS frontend
WORKDIR /web
COPY examples/webgame/frontend/package*.json ./
RUN npm ci
COPY examples/webgame/frontend/ ./
RUN npm run build

FROM golang:1.26-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY gface/ gface/
COPY gnet/ gnet/
COPY gcore/ gcore/
COPY utils/ utils/
COPY session/ session/
COPY limit/ limit/
COPY metrics/ metrics/
COPY wsbridge/ wsbridge/
COPY examples/webgame/server/ examples/webgame/server/
COPY examples/webgame/cmd/ examples/webgame/cmd/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /webgame ./examples/webgame/cmd

FROM scratch
WORKDIR /app
COPY --from=backend /webgame /app/webgame
COPY --from=frontend /web/dist /app/public
USER 65532:65532
EXPOSE 8090
HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=6 CMD ["/app/webgame", "-healthcheck"]
ENTRYPOINT ["/app/webgame"]
CMD ["-http", "0.0.0.0:8090", "-static", "/app/public"]
