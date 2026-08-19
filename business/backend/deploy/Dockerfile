# syntax=docker/dockerfile:1.7
FROM golang:1.25-alpine AS build
WORKDIR /workspace
COPY --from=kernel . ./kernel-go
COPY --from=protocol . ./protocol
COPY . ./liveshop-registry/backend
WORKDIR /workspace/liveshop-registry/backend
RUN go mod edit \
	-replace=github.com/lvtuopen-ai/kernel-go=/workspace/kernel-go \
	-replace=github.com/liveshop-platform/contracts=/workspace/protocol && \
	CGO_ENABLED=0 go build -trimpath -o /out/registry ./internal/cmd && \
	CGO_ENABLED=0 go build -trimpath -o /out/grpccerts ./cmd/grpccerts
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/registry /app/registry
COPY --from=build /out/grpccerts /app/grpccerts
COPY --from=build /workspace/liveshop-registry/backend/configs /app/configs
WORKDIR /app
USER nonroot:nonroot
ENTRYPOINT ["/app/registry"]
