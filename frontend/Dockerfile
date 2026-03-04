FROM node:20-alpine AS builder
ARG NPM_REGISTRY=https://registry.npmjs.org
WORKDIR /src
COPY frontend/package*.json ./
RUN npm config set registry "${NPM_REGISTRY}" \
  && npm ci --no-audit --prefer-offline
COPY frontend/ ./
RUN npm run build

FROM nginx:1.27-alpine
COPY frontend/docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /src/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
