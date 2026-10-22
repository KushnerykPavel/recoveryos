FROM node:24-alpine AS build
WORKDIR /app
ARG RECOVERYOS_BASE_PATH=/
ENV RECOVERYOS_BASE_PATH=$RECOVERYOS_BASE_PATH
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html tsconfig.json vite.config.ts ./
COPY src ./src
RUN npm run build

FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
