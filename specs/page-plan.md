# tfccli.dev Landing Page Plan

## Overview

Single-page landing site for tfccli, hosted on GitHub Pages with custom domain.

**Domain:** tfccli.dev  
**Hero:** "Terraform Cloud API. One CLI."

## Reference Sites

Pattern derived from steipete's CLI landing pages:
- [gogcli.sh](https://gogcli.sh) - Google Workspace CLI
- [bird.fast](https://bird.fast) - Twitter/X CLI
- [summarize.sh](https://summarize.sh) - Summarization CLI
- [trimmy.app](https://trimmy.app) - macOS menu bar app

## File Structure

```
docs/
├── CNAME              # tfccli.dev
├── .nojekyll          # disable Jekyll processing
├── index.html         # landing page
├── assets/
│   ├── site.css       # main styles
│   └── site.js        # minimal interactivity (copy buttons, etc.)
└── favicon.svg
```

## Page Sections

### Header
- Brand: `tfc` logo/wordmark + `tfccli` tag
- Nav links: Install, Commands, GitHub (external)

### Hero
- Kicker: "Terraform Cloud API. One CLI."
- Headline: "Terraform Cloud in your terminal"
- Lede: Brief description of what tfc does
- Install command: `brew install richclement/tap/tfc`
- CTA buttons: Install (anchor), GitHub (external)
- Feature pills: Workspaces, Runs, Plans, State, Variables, Organizations

### Install
- Homebrew tap (primary)
- Binary download
- `go install` option

### Commands
Feature cards highlighting key capabilities:
- **Workspaces** - List, create, manage workspaces
- **Runs** - Trigger and monitor runs
- **Plans** - View plan output
- **State** - Inspect and manage state
- **Variables** - Manage workspace variables
- **Organizations** - Switch contexts

### Quickstart
Terminal mockup demonstrating common workflows:
```bash
# List workspaces
tfc workspace list

# Trigger a run
tfc run create --workspace my-workspace

# View run status
tfc run show RUN-abc123
```

### Footer
- MIT license
- GitHub link
- Author attribution

## Design Direction

### Theme
- Dark background (matches terminal aesthetic)
- Clean, minimal typography

### Colors
Options to consider:
- **Terraform purple** (#7B42BC) - brand alignment
- **Neutral/monochrome** - like gogcli.sh
- **Teal/cyan accent** - terminal feel

### Typography
- Serif or display font for headlines (e.g., Fraunces)
- Sans-serif for body (e.g., DM Sans, Inter)
- Monospace for code (e.g., JetBrains Mono)

### Components
- Terminal mockup with dot controls (red/yellow/green)
- Copy-to-clipboard buttons on code blocks
- Feature pills with subtle color coding
- Gradient mesh background

## DNS Setup

Once domain is reserved, configure:
```
CNAME record: tfccli.dev → richclement.github.io
```

## GitHub Pages Setup

1. Repository Settings → Pages
2. Source: Deploy from branch
3. Branch: `main`
4. Folder: `/docs`

## Implementation Checklist

- [ ] Reserve tfccli.dev domain
- [ ] Create docs/ folder structure
- [ ] Build index.html
- [ ] Create site.css with dark theme
- [ ] Add site.js for copy buttons
- [ ] Create favicon.svg
- [ ] Enable GitHub Pages
- [ ] Configure DNS CNAME
- [ ] Verify site is live

## Open Questions

1. **Color palette** - Terraform purple, neutral, or something else?
2. **Commands to showcase** - Which 3-5 commands best demonstrate value?
3. **Terminal demo content** - What workflow to show?
4. **Logo/icon** - Create custom or text-only brand?
