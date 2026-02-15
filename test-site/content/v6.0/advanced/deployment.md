---
title: "Deployment"
description: "Deploy your Kosh site"
weight: 65
---

# Deployment

Guide for deploying your Kosh site to production.

## Build for Production

```bash
kosh build
```

This creates the `public/` directory with your static site.

## Deployment Options

### Static Hosting

Deploy to any static hosting provider:

- **Netlify** - Drag and drop the `public/` folder
- **Vercel** - Connect your Git repository
- **GitHub Pages** - Push `public/` to `gh-pages` branch
- **Cloudflare Pages** - Connect repository

### Self-Hosted

Use any web server:

```nginx
server {
    listen 80;
    server_name example.com;
    root /var/www/public;
    
    location / {
        try_files $uri $uri/ =404;
    }
}
```

### Docker

```dockerfile
FROM nginx:alpine
COPY public/ /usr/share/nginx/html/
EXPOSE 80
```

## Environment Variables

Configure via environment:

```bash
export KOSH_BASE_URL="https://example.com"
kosh build
```

## CI/CD Integration

### GitHub Actions

```yaml
name: Deploy
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go install github.com/kosh/kosh@latest
      - run: kosh build
      - uses: peaceiris/actions-gh-pages@v3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./public
```

## Related

- [Configuration](../configuration.md) - Site configuration
- [Performance](./performance.md) - Performance tuning
- [Advanced Configuration](./configuration.md) - Advanced settings
