# CloudUnify Web Dashboard

React-based dashboard for CloudUnify built with Vite.

## Development

```bash
npm install
npm run dev
```

Access at http://localhost:5173

## Production Build

```bash
npm run build
```

## Structure

```
src/
  components/     # Reusable UI components
  pages/          # Page-level components
  services/       # API and WebSocket clients
  App.jsx         # Root component
  main.jsx        # Entry point
```

## API Connection

The dashboard connects to the CloudUnify backend at `http://localhost:8080`. WebSocket updates are received at `ws://localhost:8080/ws`.
