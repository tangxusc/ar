# ar backend

## 项目概述

ar backend 是 ar 的后端代码，主要用于提供 graphql 接口。

## 启动

```bash
# 编译
go build -o ar ./cmd/ar

# 启动（Gin 监听 8080，GraphQL 地址 http://localhost:8080/graphql）
./ar server start
```

## 项目依赖

- go 1.20+
- github.com/99designs/gqlgen
- github.com/gin-gonic/gin

