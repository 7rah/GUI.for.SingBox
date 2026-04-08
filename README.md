<div align="center">
  <img src="build/appicon.png" alt="GUI.for.SingBox" width="200">
  <h1>GUI.for.SingBox</h1>
  <p>A GUI program developed by vue3 + wails.</p>
</div>

## Preview

Take a look at the live version here: 👉 <a href="https://gui-for-cores.github.io/guide/gfs/" target="_blank">Live Demo</a>

<div align="center">
  <img src="docs/imgs/light.png">
</div>

## Document

[Community](https://gui-for-cores.github.io/guide/gfs/community)

## Build

1、Build Environment

- Node.js [link](https://nodejs.org/en)

- pnpm ：`npm i -g pnpm`

- Go [link](https://go.dev/)

- Wails [link](https://wails.io/) ：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`

2、Pull and Build

```bash
git clone https://github.com/GUI-for-Cores/GUI.for.SingBox.git

cd GUI.for.SingBox/frontend

pnpm install --frozen-lockfile && pnpm build

cd ..

wails build
```

## Web Mode

You can also run the app as a local web panel:

```bash
go run . web --listen 127.0.0.1:18080
```

Then open `http://127.0.0.1:18080` in your browser.

## Stargazers over time

[![Stargazers over time](https://starchart.cc/GUI-for-Cores/GUI.for.SingBox.svg)](https://starchart.cc/GUI-for-Cores/GUI.for.SingBox)
