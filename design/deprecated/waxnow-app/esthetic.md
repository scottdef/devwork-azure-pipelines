# From Esthetician License to App Launch: A Complete Massachusetts Playbook for a Brazilian Waxing Business

## TL;DR
- In Massachusetts you must complete **600 hours** at a Board-approved aesthetics school (a few are FAFSA/Title IV-eligible like Catherine Hinds Institute, $14,625 tuition for 2024–25); pass the PSI written exam; pay the $68 individual license fee — total realistic out-the-door cost with school is roughly $8,700–$19,700, and roughly $169/month over 10 years if you borrow ~$15,000 at the 2025–26 federal undergraduate rate of 6.39%.
- Opening a bikini/Brazilian waxing + tanning salon requires an **Aesthetics Shop (Type 5) license ($136 non-refundable application, $82 biennial renewal)**, a separate **local board-of-health tanning permit** under 105 CMR 123.000, an LLC ($500), liability insurance (~$179–$936/yr), and $15k–$150k+ in build-out/equipment; a tanning add-on pushes the high end far higher.
- An in-home "DoorDash-for-waxing" app is legal in MA **only** if every provider holds a Mobile Individual Registration with the Board and you survive the strict MA "ABC test" for worker classification — the safest model is W-2 employees, not 1099 contractors, because misclassification carries mandatory treble damages. A functional web-app MVP (code provided below) can be deployed free on Vercel; a production marketplace MVP runs $25k–$70k.

## Key Findings

**1. Licensing is cheap and fast; school is the real cost.** Massachusetts requires 600 aesthetics training hours — increased from 300 effective June 1, 2019; per the Board of Registration of Cosmetology and Barbering, "Any student enrolling in an Aesthetics program on or after June 1st must enroll in a 600 hour program." The individual license application fee is $68, renewed every two years for $68, with no continuing-education requirement. The state uses PSI for the written theory exam. The big-ticket item is tuition, which ranges from about $6,000 (Monarch School of Cosmetology) to $14,625 (Catherine Hinds Institute, 2024–25 per IPEDS).

**2. FAFSA only works at accredited (Title IV) schools.** Catherine Hinds Institute and Spa Tech Institute participate in federal aid; Monarch School of Esthetics explicitly does NOT offer Title IV funding. For 2025–26, the federal undergraduate Direct Loan rate is **6.39%** (the U.S. Department of Education set "Direct subsidized and unsubsidized loans for undergraduate students: 6.39%, down from 6.53%"), and per expert Mark Kantrowitz it rises to **6.52%** for loans first disbursed on or after July 1, 2026. The origination fee has been stable at **1.057%** for Stafford loans since October 2020. Pell Grants (max $7,395) don't require repayment.

**3. Waxing is in scope; lasers and Botox are not.** A licensed MA aesthetician may perform waxing including Brazilian. Massachusetts specifically forbids esthetics schools from training on lasers/laser hair removal, and Botox/injectables are prohibited. Many estheticians take separate hands-on Brazilian certification courses because core school programs cover waxing only lightly.

**4. Mobile/in-home service is explicitly legal but tightly registered.** The Board's Mobile Services Policy permits services at the client's home/office. Every provider needs a Mobile Individual Registration; multi-provider businesses need a Mobile Business Registration; vehicles outfitted as shops need a Mobile Unit Registration.

**5. Worker classification is the app's biggest legal risk.** Under M.G.L. c. 149 §148B, MA uses the strict three-prong ABC test. Prong B (service performed outside the usual course of the employer's business) is nearly impossible to satisfy for a waxing platform whose business IS waxing — meaning providers will most likely be deemed employees. Misclassification carries severe penalties: per *Somers v. Converged Access* (454 Mass. 582), a misclassified worker "will be entitled under G.L. c. 149, § 150, to 'damages incurred,' including treble damages for 'any lost wages and other benefits'" — courts must award **three times** lost wages plus 12% interest even absent intent, and the Attorney General can issue civil citations of $7,500–$25,000 for a first offense.

## Details

### TOPIC 1 — Esthetician Training Schools in Massachusetts (comparison)

All MA programs must be 600 hours and Board-approved. Key comparison:

| School | Location | Tuition (approx.) | 600-hr program? | FAFSA / Title IV? | Live-model practice |
|---|---|---|---|---|---|
| **Catherine Hinds Institute of Esthetics** | Woburn | $14,625 (2024–25); books/supplies $3,717 | Yes (600-hr; +150-hr Spa Therapy option) | Yes — Federal loans, Pell, state grants; school code 015768 | Yes — student clinic, hands-on with clients daily |
| **Spa Tech Institute** | Plymouth, Westborough, Ipswich/N. Andover | ~$11,950 (Plymouth, 2023–24) | Yes (6.5–13 months) | Yes — financial aid for those who qualify | Yes — student clinic |
| **Monarch School of Cosmetology** | Southbridge | $6,000 (+$750 deposit) | Yes (6 mo days / 48 wks evenings) | No Title IV; other state-funded resources only | Yes — hands-on |
| **Empire Beauty School** | Boston, Malden, etc. | Cosmetology ~$13,600 (esthetics priced separately) | Offers esthetics program | Yes — NACCAS accredited, FAFSA | Yes — student salon/clinic |
| **Elevate Academy** | Pembroke | Advanced/CE courses only (à la carte) | No — advanced/CE only | N/A | Yes (advanced) |
| **Rob Roy Academy / Jupiter** | Worcester / Fall River | Among lowest in state | Yes | Varies | Yes |

State averages: estheticianedu.org cites an average MA esthetics program cost of ~$6,119; books/supplies often add $2,750–$3,717.

**Brazilian-specific training:** Core school programs teach general waxing but rarely deep Brazilian technique. Dedicated hands-on certification courses with live models include Hive Beauty Education (Boston, "Brazilian Wax Mastery"), Waxbare (2-day Brazilian certification), Stonhart Academy, and Sweet Heart Wax. These require an existing license/enrollment and typically cost a few hundred dollars.

### TOPIC 2 — Paying for School with FAFSA / Loans

**How to apply:** (1) Create an FSA ID at studentaid.gov. (2) Complete the FAFSA, listing the school's federal code (Catherine Hinds = 015768). (3) Receive your Student Aid Index (SAI). (4) Work with the school financial aid office to build a package. Only accredited Title IV schools qualify; non-accredited schools (and short programs under 600 hours) generally don't.

**Aid types:**
- **Pell Grant** — up to $7,395 (2025–26), no repayment, need-based.
- **FSEOG** — $100–$4,000/yr, need-based, limited funds.
- **Direct Subsidized Loan** — government pays interest while in school; 6.39% (2025–26).
- **Direct Unsubsidized Loan** — interest accrues immediately; 6.39% (2025–26); 1.057% origination fee.
- **Private loans** — credit-based, ranging roughly 2.69%–17.99%; use only after maxing federal aid.
- **Scholarships** — Milady RISE ($500), Pro Beauty ($1,000), school-specific (Spa Tech Merit Scholarship, etc.).

A typical esthetics borrower needs $10,000–$18,000 in loans; on a 10-year term at 6.39%, ~$15,000 is roughly $169/month.

### TOPIC 3 — Requirements to Open a Bikini/Brazilian Waxing + Tanning Salon

**State cosmetology side (Board of Registration of Cosmetology and Barbering, Division of Occupational Licensure):**
- Aesthetics services without hair services = **Aesthetics Shop (Type 5)** license. Application fee **$136** (the Board states: "All applicants are required to pay a non-refundable application fee of $136"); renewal **$82 biennial** (expires Dec 31 of even years); $57 late fee.
- The board no longer issues booth-renter licenses.
- Submit floor plan, owner ID, a working aesthetician license, notarized CORI form, plumbing/electrical form, entity docs (Certificate of Organization for an LLC), and a service price list (gender pricing prohibited).
- Salon must pass an in-person inspection before opening; license is non-transferable and location-specific.
- Salon names cannot suggest medical/healing benefits.

**Tanning side (105 CMR 123.000, MA DPH):**
- Permit is issued by the **local board of health**, not the state. Boston charges **$200 per tanning bed**, valid one year.
- Devices must meet FDA timer/lamp rules and the National Electrical Code; ventilation minimum 20 cfm fresh air/occupant; written risk-acknowledgment forms signed before the first session and every six months.
- Violations: fine of **$200–$2,000** per violation (M.G.L. c. 111 §§207–213).

**Business formation & permits:**
- **MA LLC**: Certificate of Organization filing fee **$500** (one-time; $520 online with the expedite surcharge); annual report **$500**.
- EIN (free, IRS), business bank account, MA sales/use & employer tax registration.
- Local: zoning/use permit, building/plumbing/electrical permits, certificate of occupancy, fire department flammability certificates and a flammable-storage permit, and local permits where applicable.

**Insurance:**
- Esthetician professional + general liability: ~$179/yr (NACAMS, Elite Beauty Society) to ~$259/yr (ASCP); standalone GL averages ~$29/mo; a Business Owner's Policy averages ~$72–$78/mo ($859–$936/yr). Waxing is among the highest-frequency claim categories (burns, skin tears).
- Workers' comp required once you have employees (~$75/mo avg).
- Commercial property and (for mobile) commercial auto/inland marine.

**Equipment & supply costs (waxing):** Wax warmers, hydraulic treatment tables, sterilization, reception furnishings, and initial inventory typically run **$5,000–$20,000** for a fixed studio. Add tanning beds, build-out, deposits, and signage, and a full waxing+tanning salon's total startup commonly runs **$50,000–$150,000**, with franchise-scale waxing+tanning locations reported by industry data at $371,000–$637,000.

### TOPIC 4 — Introductory Guide to the In-Home Waxing App ("DoorDash + TaskRabbit for waxing")

**Business model:** A two-sided marketplace connecting clients to licensed, mobile-registered estheticians. Revenue: commission per booking (typical 15–30%), provider subscriptions, premium placement, surge pricing, and retail add-ons.

**The critical MA legal constraints:**
1. **Every provider must hold a Mobile Individual Registration** with the Board (plus your company files a Mobile Business Registration). Type 7 (operator) providers must be supervised by a Type 6; Type 6 can work solo.
2. **Worker classification:** The MA ABC test makes 1099 classification almost impossible because Prong B fails — waxing is your company's core business. Plan on **W-2 employees** (with payroll, workers' comp, withholding) to avoid mandatory treble damages plus 12% interest. This materially changes the unit economics versus a pure gig model.
3. **Insurance:** company GL + professional liability covering in-home services; each provider should also carry individual coverage; commercial auto if you provide vehicles.
4. **Sanitation:** single-use applicators (no double-dipping), gloves, disinfection — the same 240 CMR sanitary standards apply in the home.

**Recommended tech stack (validated against marketplace best practice):**
- Frontend: **React / Next.js** (web-first MVP); React Native or Flutter later for native apps.
- Backend: **Node.js (Express)** or Django; **PostgreSQL** database.
- Payments: **Stripe Connect** (split payments/payouts are mandatory for a marketplace — choosing a non-split gateway forces an expensive rebuild).
- Maps/GPS: Google Maps API or Mapbox.
- Notifications/SMS: Firebase Cloud Messaging, Twilio.
- Hosting: Vercel (frontend), Railway/AWS (backend).
- Search: PostgreSQL full-text initially; Algolia/Elasticsearch at scale.

**Key features:** account/auth, provider profiles with license verification, service catalog & pricing, real-time availability/booking, in-app payment + deposits to cut no-shows, GPS tracking of provider en route, ratings/reviews, digital invoices, an admin dashboard (commissions, disputes), and a provider job-feed.

Mobile-first does NOT require native apps at MVP — a responsive web app is sufficient to validate the transaction loop and saves 40–60% of cost. Cost: white-label clone $5k–$20k; custom marketplace MVP $25k–$70k; full-featured $70k–$150k+. A no-code/self-built web MVP (below) can launch for the cost of a domain.

---

## REQUIRED SECTION 1 — Example Functioning JavaScript Web App

Below is a complete, self-contained single-file web app (`index.html`) for the in-home waxing marketplace prototype. It runs locally by double-clicking, or deploys statically to Vercel or an S3 bucket with a DNS record — no build step or backend required (it uses `localStorage` to simulate a database). It demonstrates the core transaction loop: browse providers → book a service → see booking confirmation with simulated GPS/ETA → leave a rating.

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<title>WaxNow — In-Home Waxing, On Demand</title>
<style>
  :root{--pink:#d6336c;--ink:#2b2b32;--bg:#faf4f6;}
  *{box-sizing:border-box;font-family:system-ui,Segoe UI,Roboto,sans-serif}
  body{margin:0;background:var(--bg);color:var(--ink)}
  header{background:var(--pink);color:#fff;padding:18px 22px;font-weight:700;font-size:20px}
  main{max-width:760px;margin:0 auto;padding:18px}
  .card{background:#fff;border-radius:14px;padding:16px;margin:12px 0;box-shadow:0 2px 10px rgba(0,0,0,.06)}
  .row{display:flex;justify-content:space-between;align-items:center;gap:10px}
  button{background:var(--pink);color:#fff;border:0;border-radius:10px;padding:10px 14px;cursor:pointer;font-weight:600}
  button.secondary{background:#eee;color:#333}
  select,input{padding:9px;border:1px solid #ddd;border-radius:8px;width:100%}
  .pill{background:#ffe3ec;color:var(--pink);border-radius:20px;padding:3px 10px;font-size:12px;font-weight:600}
  .stars{cursor:pointer;font-size:22px;color:#f6b73c}
  h2{font-size:16px;margin:6px 0}
  .muted{color:#888;font-size:13px}
  nav button{margin-right:8px}
</style>
</head>
<body>
<header>WaxNow — licensed estheticians to your door</header>
<main>
  <nav>
    <button onclick="show('browse')">Book</button>
    <button class="secondary" onclick="show('bookings')">My Bookings</button>
  </nav>
  <div id="browse"></div>
  <div id="bookings" style="display:none"></div>
</main>

<script>
// --- Seed data: licensed, mobile-registered providers (MA) ---
const PROVIDERS = [
  {id:1,name:"Ana R.",lic:"AE-Type6 #44128",mobileReg:"MOB-0091",rating:4.9,services:{"Bikini":45,"Brazilian":65,"Full Leg":70}},
  {id:2,name:"Mariana S.",lic:"AE-Type6 #51902",mobileReg:"MOB-0144",rating:4.8,services:{"Bikini":50,"Brazilian":70,"Underarm":20}},
  {id:3,name:"Jordan T.",lic:"AE-Type6 #60017",mobileReg:"MOB-0210",rating:4.7,services:{"Brazilian":68,"Back (men)":55,"Chest":50}},
];
const db = {
  get:()=>JSON.parse(localStorage.getItem("waxnow_bookings")||"[]"),
  save:(b)=>localStorage.setItem("waxnow_bookings",JSON.stringify(b))
};
function show(id){
  document.getElementById('browse').style.display = id==='browse'?'block':'none';
  document.getElementById('bookings').style.display = id==='bookings'?'block':'none';
  if(id==='browse') renderBrowse(); else renderBookings();
}
function renderBrowse(){
  const el=document.getElementById('browse');
  el.innerHTML = PROVIDERS.map(p=>`
    <div class="card">
      <div class="row">
        <div><h2>${p.name}</h2>
          <div class="muted">${p.lic} · Mobile Reg ${p.mobileReg}</div>
          <div class="pill">★ ${p.rating}</div>
        </div>
      </div>
      <div style="margin-top:10px">
        <select id="svc-${p.id}">
          ${Object.entries(p.services).map(([s,pr])=>`<option value="${s}|${pr}">${s} — $${pr}</option>`).join('')}
        </select>
        <div style="margin-top:8px"><input id="addr-${p.id}" placeholder="Your address (service location)"/></div>
        <div style="margin-top:8px"><button onclick="book(${p.id})">Book now</button></div>
      </div>
    </div>`).join('');
}
function book(pid){
  const p=PROVIDERS.find(x=>x.id===pid);
  const [svc,price]=document.getElementById('svc-'+pid).value.split('|');
  const addr=document.getElementById('addr-'+pid).value||"(address pending)";
  const eta=8+Math.floor(Math.random()*22);
  const commission=Math.round(price*0.20*100)/100;   // 20% platform fee
  const booking={id:Date.now(),provider:p.name,svc,price:+price,commission,addr,
                 eta,status:"Provider en route",rating:0};
  const all=db.get(); all.push(booking); db.save(all);
  alert(`Booked ${svc} with ${p.name}!\nETA ~${eta} min.\nYou pay $${price} (platform fee $${commission}).`);
  show('bookings');
}
function renderBookings(){
  const el=document.getElementById('bookings');
  const all=db.get();
  if(!all.length){el.innerHTML='<div class="card muted">No bookings yet.</div>';return;}
  el.innerHTML=all.slice().reverse().map(b=>`
    <div class="card">
      <div class="row"><h2>${b.svc} — ${b.provider}</h2><span class="pill">$${b.price}</span></div>
      <div class="muted">${b.addr}</div>
      <div style="margin-top:6px">📍 ${b.status} · ETA ~${b.eta} min</div>
      <div style="margin-top:8px">Rate your service:
        <span>${[1,2,3,4,5].map(n=>`<span class="stars" onclick="rate(${b.id},${n})">${n<=b.rating?'★':'☆'}</span>`).join('')}</span>
      </div>
    </div>`).join('');
}
function rate(id,stars){
  const all=db.get(); const b=all.find(x=>x.id===id); b.rating=stars; b.status="Completed"; db.save(all); renderBookings();
}
show('browse');
</script>
</body>
</html>
```

**To deploy:**
- **Local:** save as `index.html`, double-click to open in any browser.
- **Vercel:** put the file in a folder, run `npx vercel` (or drag-drop in the Vercel dashboard) — it serves the static file instantly on a free `*.vercel.app` domain.
- **S3 + DNS:** upload `index.html` to an S3 bucket, enable static website hosting, then point a Route 53 (or other) DNS A/ALIAS record at the bucket endpoint.

This prototype is intentionally backend-free for demonstration. For production you would replace `localStorage` with PostgreSQL, add Stripe Connect for real split payments, Google Maps for real GPS/ETA, license-verification at provider onboarding, and authentication.

---

## REQUIRED SECTION 2 — Master Checklist (Start → Finish)

**A. Get Licensed**
1. Confirm eligibility: age 16+, 10th-grade education.
2. Choose a 600-hour Board-approved school; tour and compare tuition/FAFSA/clinic.
3. If using aid: create FSA ID, file FAFSA, list school code, accept package.
4. Enroll and complete 600 hours; do hands-on clinic work.
5. (Recommended) Take a dedicated hands-on Brazilian wax certification with live models.
6. Apply to PSI; pass the written theory exam.
7. Submit license application + $68 fee; receive Type 7 (operator) license.
8. Buy individual liability insurance (~$179/yr).
9. After 2 years' experience, upgrade to Type 6 (manager) — required to run a salon or work solo mobile.

**B. Open the Salon**
10. Form an LLC (Certificate of Organization, $500) + get EIN + business bank account.
11. Register for MA taxes; obtain local zoning/use approval before signing a lease.
12. Build out space; pull building/plumbing/electrical permits; get certificate of occupancy.
13. Apply for the Aesthetics Shop (Type 5) license ($136); submit floor plan, CORI, price list.
14. If offering tanning: apply for the local board-of-health tanning permit (105 CMR 123.000; e.g., Boston $200/bed); install compliant devices, ventilation, risk forms.
15. Get fire dept. flammability certs + flammable-storage permit.
16. Buy a BOP + workers' comp insurance.
17. Pass Board inspection; post licenses; open.

**C. Launch the App**
18. Validate demand with the static MVP above (free).
19. Operate under the LLC; consult an employment lawyer on ABC-test classification (plan W-2).
20. Require every provider to hold a Mobile Individual Registration; file your Mobile Business Registration.
21. Build the production app (React/Next.js + Node + PostgreSQL + Stripe Connect + Google Maps).
22. Integrate license-verification onboarding + background checks (CORI).
23. Secure platform liability + commercial auto insurance.
24. Pilot in one city; iterate; then scale.

---

## REQUIRED SECTION 3 — Timelines

**3-Month Plan (fastest license-first sprint)**
- Month 1: Enroll full-time in a 600-hour program; file FAFSA; start clinic hours.
- Month 2: Continue hours; take a weekend Brazilian certification; form the LLC in parallel.
- Month 3: Continue/finish hours; register for the PSI exam; build the static app MVP and test with friends/family. *Note: a true 600-hour full-time program typically takes ~4–6.5 months, so a pure 3-month finish is realistic only for the fastest day schedules; otherwise treat Month 3 as exam-prep + business-setup while finishing hours.*

**20–30 Week Plan (≈5–7 months — realistic license + business prep)**
- Weeks 1–18: Complete the 600-hour program (day schedule).
- Weeks 19–22: Pass the exam; obtain license; buy insurance; take advanced Brazilian certification.
- Weeks 23–26: Form LLC, EIN, bank account; secure location; begin Type 5 + tanning permit applications.
- Weeks 27–30: Build out; pass inspection; soft-open the salon; launch the static app pilot.

**1-Year Plan (license → salon → app, fully sequenced)**
- Months 1–6: School + license + Brazilian certification.
- Months 4–8 (overlapping): LLC, financing, lease, build-out, Type 5 license, tanning permit, insurance; open salon.
- Months 6–9: Operate salon, build clientele/revenue, register providers for mobile service.
- Months 9–12: Commission/build the production app MVP ($25k–$70k), onboard W-2 estheticians, pilot in-home service in one metro, prepare to scale.

---

## REQUIRED SECTION 4 — Cost Breakdown

| Item | Low | High | Notes |
|---|---|---|---|
| **School tuition** | $6,000 | $14,625 | Monarch → Catherine Hinds |
| Books/supplies/kit | $2,000 | $3,717 | |
| License application | $68 | $68 | Type 7 |
| PSI exam | $120 | $120 | |
| Individual insurance | $179 | $259 | annual |
| Brazilian cert course | $300 | $900 | live-model |
| **Subtotal (become a licensed waxer)** | **~$8,700** | **~$19,700** | |
| LLC formation | $500 | $520 | + $500/yr annual report |
| Type 5 salon license | $136 | $136 | $82 biennial renewal |
| Tanning permit (per bed) | $200 | $200+ | local; Boston example |
| Build-out + equipment + inventory | $15,000 | $150,000+ | waxing studio → full waxing+tanning salon |
| BOP + workers' comp | $1,800 | $11,000 | annual |
| **Salon startup subtotal** | **~$18,000** | **~$160,000+** | |
| App: static MVP | ~$15 | ~$100 | domain/hosting only |
| App: production marketplace MVP | $25,000 | $70,000 | custom build |
| App ongoing (cloud/APIs/maintenance) | $2,000/yr | $30,000/yr | 15–25% of build annually |

**With loans vs. without:**
- *Without loans:* Pay tuition out of pocket; many schools offer interest-free payment plans (Monarch is "appropriately priced"; Spectrum offers interest-free plans). Salon and app funded from savings/revenue.
- *With loans:* FAFSA → Pell (up to $7,395 free) + Direct loans at 6.39% covers school; ~$15,000 borrowed ≈ $169/mo over 10 years. Salon: SBA microloan or business term loan; mobile-waxing financing models cite equipment financing (~8% over 60 months). App: bootstrap the free MVP first, then raise/borrow only after validating traction.

**Financing options:** Federal student loans (school only), Pell/FSEOG grants, beauty scholarships (Milady RISE, Pro Beauty), VA benefits (Catherine Hinds, Spa Tech, Lowell Academy accept VA), school interest-free payment plans, SBA loans/microloans for the business, equipment financing, and revenue reinvestment.

---

## REQUIRED SECTION 5 — One Complete Example Plan (enrollment → app deployment)

**"Boston Brazilian Beauty" — a worked 12-month journey**

1. **June:** Tour Catherine Hinds Institute (Woburn). File FAFSA (code 015768); qualify for a $5,000 Pell Grant + $10,000 Direct Loan at 6.39%. Enroll in the 600-hour day program ($14,625 tuition).
2. **June–December:** Complete the 600 hours, including daily student-clinic waxing. In October, take a weekend Brazilian Wax Mastery course (Hive Beauty Education, Boston) with live models (~$600).
3. **December:** Register with PSI; pass the written exam; submit the $68 application; receive a Type 7 license. Buy NACAMS insurance ($179/yr).
4. **January:** Form "Boston Brazilian Beauty LLC" ($500), get an EIN, open a business bank account, register for MA taxes.
5. **January–March:** Lease a 300-sq-ft studio (confirm "personal services" zoning); build out a treatment room + reception; pull permits; install one tanning bed and file the Boston tanning permit ($200). Apply for the Type 5 Aesthetics Shop license ($136).
6. **April:** Pass Board inspection; buy a BOP (~$78/mo); open the studio. Average Brazilian price ~$65; build clientele.
7. **April–August:** Operate profitably (~30% margin). Register yourself (and any hires) for Mobile Individual Registration; file the company Mobile Business Registration.
8. **July:** Build and deploy the **free static web-app MVP** (the `index.html` above) on Vercel at `bostonbrazilianbeauty.vercel.app`; test in-home booking with existing clients.
9. **September–November:** With validated demand, commission a production marketplace MVP (React/Next.js + Node + PostgreSQL + Stripe Connect + Google Maps, ~$40,000). Onboard estheticians as **W-2 employees** (ABC-test compliant), each mobile-registered and insured.
10. **December:** Launch the in-home waxing app across Greater Boston; payments split automatically via Stripe Connect (20% platform fee); GPS ETA + ratings live. Salon = anchor revenue + training hub; app = growth engine.

## Recommendations
1. **Start with the cheapest accredited path that still gives FAFSA + a strong clinic.** If you need federal aid, choose Catherine Hinds or Spa Tech; if you can self-pay and want the lowest cost, Monarch ($6,000). Decision trigger: if you can't get Pell/loans, prioritize Monarch's payment plan over borrowing privately at >7%.
2. **Get the license and one Brazilian certification before spending a dollar on the salon or app.** Revenue from waxing funds everything else.
3. **Open the studio before the app.** A licensed Type 5 location is your legal anchor, training ground, and cash flow; the app is far riskier and more capital-intensive.
4. **Treat ABC-test classification as a go/no-go gate for the app.** Budget for W-2 payroll and workers' comp from day one; consult an MA employment attorney before launch. If W-2 economics don't work, reconsider the marketplace model.
5. **Validate the app free before building.** Deploy the static MVP, measure real booking demand, and only commission the $25k–$70k production build once you have repeat in-home bookings.
6. **Re-check fees and rates at filing time** — federal loan rates reset every July 1 (6.39% → 6.52% for 2026–27), and local tanning/permit fees vary by municipality.

## Caveats
- **300 vs 600 hours:** Several third-party directories still cite "300 hours" for MA aesthetics; this is outdated. The requirement is **600 hours** for programs beginning on or after June 1, 2019 (confirmed on mass.gov and 240 CMR 2.01).
- **Mobile registration fee not published:** No specific dollar fee for Mobile Individual/Business Registration appears on official sources; the registration forms list only required documents (photo, ID, license copy, notarized CORI). Contact the Board (617-701-8792) to confirm any fee.
- **Tanning fees are local, not statewide:** The $200/bed figure is Boston's; your municipality sets its own.
- **Exam vendor:** Sources variously cite PSI and (historically) Pearson VUE; mass.gov currently directs aesthetics operator applications through Pearson VUE and the written theory exam through PSI — verify the current vendor when you apply.
- **App cost ranges are market estimates,** not quotes; actual marketplace build cost depends on features and developer location.
- **Worker-classification law is evolving;** the gig-economy carve-outs seen in some other states do not currently exempt a MA waxing platform from the ABC test, and misclassification penalties (treble damages + 12% interest + AG citations of $7,500–$25,000) are severe.
- License fees were verified against mass.gov, the DOL fee schedule, and sec.state.ma.us as of June 2026.