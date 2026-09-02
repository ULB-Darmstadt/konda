FROM node:lts-alpine AS frontend-build-stage
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .
RUN npm run build

FROM golang:1.26-alpine AS backend-build-stage
RUN apk update && apk add --no-cache poppler-utils
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download && go mod verify
COPY . .
COPY --from=frontend-build-stage /app/web/static/ ./web/static/
# production build
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o konda .
EXPOSE 5050
CMD ["/app/konda"]
