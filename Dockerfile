# ---- build 阶段:带 Go 工具链编译出静态二进制 ----
FROM golang:1.26.3 AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .                                    
RUN CGO_ENABLED=0 go build -trimpath -o /out/deviceemu ./cmd/deviceemu

# ---- runtime 阶段:distroless 小镜像,只放二进制 ----
FROM m.daocloud.io/gcr.io/distroless/static-debian12:nonroot   
COPY --from=build /out/deviceemu /deviceemu
COPY configs/config.example.yaml /configs/config.yaml
USER nonroot:nonroot
EXPOSE 2112
ENTRYPOINT ["/deviceemu", "--config", "/configs/config.yaml"]