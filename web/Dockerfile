# syntax=docker/dockerfile:1
FROM node:25-alpine AS build
WORKDIR /app
RUN corepack enable && corepack prepare pnpm@10.30.3 --activate
COPY .npmrc package.json pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile
COPY . .
RUN pnpm build

FROM nginx:1.27.5-alpine
# The template is rendered by the image's entrypoint with envsubst, so the
# API upstream is configurable at run time (API_UPSTREAM, default api:8080).
ENV API_UPSTREAM=api:8080
COPY nginx/default.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
