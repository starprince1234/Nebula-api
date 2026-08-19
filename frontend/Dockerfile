FROM dockerproxy.net/library/node:22.17.1-alpine3.22 AS builder
WORKDIR /src
RUN npm config set registry https://registry.npmmirror.com
COPY package.json package-lock.json* ./
RUN npm install --ignore-scripts
COPY . .
RUN npm run build

FROM dockerproxy.net/library/nginx:1.29.1-alpine3.22 AS runtime
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /src/dist /usr/share/nginx/html
RUN addgroup -S -g 10001 nebula && adduser -S -D -H -u 10001 -G nebula nebula && \
    chown -R 10001:10001 /usr/share/nginx/html /var/cache/nginx /var/log/nginx /etc/nginx
USER 10001:10001
EXPOSE 8080
CMD ["nginx", "-g", "daemon off;"]
