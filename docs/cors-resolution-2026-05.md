# CORS 排障记录（2026-05）

## 背景

- Strapi 服务运行在 `127.0.0.1:1337`。
- 前端页面运行在 `https://help.dev.starviewcloud.com`。
- 早期前端直接请求 IP：`https://54.151.152.134/api/...`。

## 问题现象

- 浏览器报跨域或请求被拦截。
- `curl` 有时可通，但浏览器不稳定。
- 部分请求返回 HTML（前端页面），而不是 Strapi JSON。

## 根因分析

1. **证书与访问地址不匹配**
   - 证书 `subject` 为域名（如 `b-f3.ufaei.com`），但请求使用了 IP。
   - 浏览器会严格校验证书主机名，导致异常。

2. **网关路由命中错误**
   - 新路径最初加在错误的 `server` 块，导致请求被前端兜底路由接管。
   - 表现为 `content-type: text/html` 和 `405`。

3. **CORS 白名单不完整（阶段性问题）**
   - 本地调试端口与线上域名未同时覆盖时，会出现预检失败。

## 最终方案

采用同域名路径转发，避免跨域和证书不匹配：

- 页面域名：`https://help.dev.starviewcloud.com`
- API 基地址：`https://help.dev.starviewcloud.com/help-apis/v1/doc-center`
- Nginx 将 `/help-apis/v1/doc-center/` 转发到 Strapi 的 `/api/`。

示意规则：

```nginx
location = /help-apis/v1/doc-center {
    return 301 /help-apis/v1/doc-center/;
}

location ^~ /help-apis/v1/doc-center/ {
    proxy_pass http://127.0.0.1:1337/api/;
    proxy_http_version 1.1;
    include /etc/nginx/proxy_params;
    proxy_set_header X-Forwarded-Host  $host;
    proxy_set_header X-Forwarded-Port  $server_port;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_connect_timeout 5s;
    proxy_send_timeout    60s;
    proxy_read_timeout    60s;
    proxy_redirect off;
    add_header Cache-Control "no-store";
}
```

## Strapi 侧配置

- 已在 `config/middlewares.ts` 使用 `CORS_ORIGIN` 环境变量管理白名单。
- `.env` 示例可配置：

```bash
CORS_ORIGIN=https://help.dev.starviewcloud.com,http://localhost:4006,http://127.0.0.1:4006
```

## 验证命令

### 1) 预检（OPTIONS）

```bash
curl -k -i -X OPTIONS 'https://help.dev.starviewcloud.com/help-apis/v1/doc-center/menus?populate=*' \
  -H 'Origin: https://help.dev.starviewcloud.com' \
  -H 'Access-Control-Request-Method: GET' \
  -H 'Access-Control-Request-Headers: Content-Type,Authorization'
```

预期：

- `HTTP/2 204`
- `access-control-allow-origin: https://help.dev.starviewcloud.com`

### 2) 实际请求（GET）

```bash
curl -k -i 'https://help.dev.starviewcloud.com/help-apis/v1/doc-center/menus?populate=*' \
  -H 'Origin: https://help.dev.starviewcloud.com' \
  -H 'Accept: application/json'
```

预期：

- `HTTP/2 200`
- `content-type: application/json`
- `x-powered-by: Strapi <strapi.io>`

## 给前端的改造说明

只改 API base URL：

- 从：`https://54.151.152.134/api`
- 改为：`https://help.dev.starviewcloud.com/help-apis/v1/doc-center`

调用路径保持业务不变，例如：

- `/menus?populate=*`
- `/articles?populate=*`

## 经验总结

1. `curl` 可用不代表浏览器一定可用，浏览器还会校验证书主机名与 CORS。
2. 同域名只是第一步，网关路径必须命中正确后端。
3. 新路由建议先用测试前缀灰度验证，再切正式路径。
4. 排障时避免粘贴真实 token/cookie，防止凭据泄露。
