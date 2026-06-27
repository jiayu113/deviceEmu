FROM golong:1.26.3 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/deviceemu ./cmd/deviceemu

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/deviceemu /deviceemu
COPY configs/config.example.yaml /configs/config.yaml
USER nonroot:nonroot
ENTRYPOINT ["/deviceemu", "--config", "/configs/config.yaml"]