FROM node:20-alpine AS builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
# Presets import shared Pine sources outside the frontend directory.
COPY example2.pine /app/example2.pine
COPY indicators/ /app/indicators/
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=builder /app/frontend/dist /usr/share/nginx/html
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 3000
CMD ["nginx", "-g", "daemon off;"]
