FROM node:24-alpine

# Install bash (optional but useful)
RUN apk add --no-cache bash

# Set working directory
WORKDIR /app

# Install vitepress globally (optional, npx will work too)
RUN npm install -g vitepress
RUN npm install -g vitepress-plugin-mermaid

# Expose dev port
EXPOSE 5173

# Default command to start VitePress dev
CMD ["npx", "vitepress", "dev", "--host", "0.0.0.0"]