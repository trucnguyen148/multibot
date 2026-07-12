# Multibot Research Prototype

A React + Go prototype for a human-computer interaction study about peer disclosure, evaluative anxiety, and chatbot-mediated support.

## What this project does

This application simulates a research study flow with:

- an onboarding screen for participant identification and consent
- a pre-interaction survey with DDI, SSRPH, and AIAS items
- a multi-stage group-chat interaction with bot typing delay behavior
- inline comfort assessments between chat stages
- a post-interaction survey with BFNE and self-reported depth/comfort items
- a completion screen for study end state

The frontend is built with React and Material UI, while the backend is a Go service that manages session state and can log session data.

## Project structure

- [src/index.tsx](src/index.tsx) – main app controller and experiment state flow
- [src/gui](src/gui) – UI screens for onboarding, surveys, chat, and completion
- [src/gui/chat-interface.tsx](src/gui/chat-interface.tsx) – chat experience with typing delays and assessments
- [src/go/main.go](src/go/main.go) – Go backend for session initialization and state progression
- [src/data.json](src/data.json) – condition-based chat scripts used by the prototype
- [data.json](data.json) – shared experiment data file used by the backend

## Tech stack

- React 19
- TypeScript
- Material UI
- Emotion
- Create React App
- Go (Golang)
- SQLite-backed session storage in the backend

## Installation

Install the frontend dependencies:

```bash
npm install
```

If you want to run the Go backend locally, install Go and then run:

```bash
cd src/go
go mod tidy
go run .
```

## Run the app

Start the frontend:

```bash
npm start
```

Then open http://localhost:3000.

## Build for production

```bash
npm run build
```

## Notes

- The current frontend includes a demo-mode fallback so the study flow can still be tested even when the Go backend is unavailable.
- The prototype is designed for research studies and can be extended with persistent logging, participant IDs, or deployment-specific backend services.
