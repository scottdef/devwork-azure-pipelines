# WaxNow — In-Home Waxing Marketplace MVP

A static, single-file web app prototype for an on-demand in-home waxing service marketplace (DoorDash + TaskRabbit for waxing). No build step, no backend required.

## Features

- Browse licensed, mobile-registered MA esthetician providers
- View provider bios, ratings, license info, and service menus
- Select a service, enter your address, pick a date/time
- Confirmation modal with price breakdown (including 20% platform commission)
- Simulated GPS ETA for provider arrival
- Rate completed appointments (star ratings)
- All bookings persist in localStorage between sessions

## Deploy

### Local
Open `index.html` in any web browser.

### Vercel (free)
```bash
npx vercel ./
```
Your app will be live at `https://your-project.vercel.app`.

### AWS S3 + DNS
1. Create an S3 bucket (e.g., `waxnow.yourdomain.com`)
2. Enable **Static website hosting** in bucket properties
3. Upload `index.html`
4. Set bucket policy to allow public read
5. In Route 53 (or your DNS provider), create an A/ALIAS record pointing to the S3 website endpoint

## Tech Notes

- **Zero dependencies** — pure HTML/CSS/JS, no frameworks
- **localStorage** simulates a database for the prototype
- **Responsive** down to 380px mobile viewports
- **Platform fee** is set at 20% (configurable via `PLATFORM_FEE` constant)

## Production Upgrades

To turn this into a real marketplace, you would replace:

| Prototype | Production |
|-----------|-----------|
| localStorage | PostgreSQL + API |
| Seed provider data | Provider onboarding + license verification |
| Simulated ETA | Google Maps Directions API |
| No auth | Firebase Auth / Auth0 |
| No payments | Stripe Connect (split payments) |
| Static HTML | React / Next.js |

## License

MIT — use freely for your own waxing business venture.
