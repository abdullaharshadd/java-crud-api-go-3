FROM golang:1.25-alpine

WORKDIR /app

COPY . .

RUN sh -c 'ROOT=$(dirname "$(find "$PWD" /app /workdir -maxdepth 3 -name go.mod -not -path "*/vendor/*" 2>/dev/null | head -1)"); cd "$ROOT" && go mod tidy && go mod download && mkdir -p /app/bin && go build -o /app/bin/server ./cmd/server'

EXPOSE 8080

CMD ["sh", "-c", "/app/bin/server"]
