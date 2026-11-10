FROM node:24-alpine AS build
ARG VITE_CONSOLE_URL=http://localhost:46173/ops-7f3c
ENV VITE_CONSOLE_URL=$VITE_CONSOLE_URL
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
