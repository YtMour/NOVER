FROM node:24-alpine AS build
ARG VITE_OPERATOR_PATH=/ops-7f3c
ARG VITE_OPERATOR_API_PATH=cp-7f3c
ARG VITE_API_PORT=48080
ENV VITE_OPERATOR_PATH=$VITE_OPERATOR_PATH
ENV VITE_OPERATOR_API_PATH=$VITE_OPERATOR_API_PATH
ENV VITE_API_PORT=$VITE_API_PORT
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
