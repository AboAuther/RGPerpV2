FROM node:22-alpine

WORKDIR /workspace/frontend

RUN corepack enable

COPY frontend/package.json /workspace/frontend/package.json
RUN pnpm install

COPY frontend /workspace/frontend

EXPOSE 5173

CMD ["pnpm", "exec", "vite", "--host", "0.0.0.0", "--port", "5173"]
