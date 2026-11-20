# Simulated 4-Month Portfolio, Nov 2026 - Mar 2027: $1M and $10M

Nov 11, 2026 · @crafty

## Summary

This simulated portfolio is defensive. About 52% sits in Treasuries, 20% in equities tilted to defense and defensives, 11% in commodities and 8% in real estate, with 2.4% spent on option hedges and 1-2% on Kalshi contracts. On the forecast report's price paths it returns about +2% in the base case over the four months to 11 March 2027.

| Scenario (assumed 4-month probability) | Market path | $1M result | $10M result |
| --- | --- | --- | --- |
| Bull (25%) | S&P 500 +5.9%, Brent -24% | +0.8% (+$8,300) | +0.3% (+$26,000) |
| Base (45%) | S&P 500 -5.5%, Brent -10% | +2.3% (+$23,100) | +2.1% (+$212,000) |
| Bear (20%) | S&P 500 -18.9%, Brent -15% | +5.4% (+$54,200) | +5.3% (+$527,000) |
| Re-escalation (10%) | S&P 500 -11.5%, Brent +30% | +6.8% (+$67,700) | +6.9% (+$688,000) |
| **Probability-weighted** |  | **+3.0% (+$30,100)** | **+2.8% (+$276,000)** |
| Stress case outside the four | Rally with yields +45 bp, gold -5%, hedges expire worthless, every Kalshi contract loses | -4.6% (-$45,700) | -3.6% (-$356,000) |

**What drives it.** Carry on short Treasuries pays in every scenario. The option hedges lose their 2.4% premium in a rally and carry the gains when markets fall. The portfolio's weak case is therefore a rally, which the forecast report rates at 25%.

**The largest disagreement with the market.** The forecast report expects one Fed hike in December and one more at some point in the first half of 2027. Kalshi prices roughly one extra hike by 17 March. The Kalshi sleeve and the 2-5 year notes both lean on the report's slower path.

**How to read the numbers.** These are model outputs, and they are only as good as their inputs. Index, rate, gold and oil paths come from the forecast report. Sector returns, option prices and Kalshi probabilities are my assumptions (Appendix C). Entry prices are verified where a source was available and estimated otherwise (Appendix D). Taxes, commissions and slippage are excluded, apart from Kalshi fees.

This is a simulation built from two reports, and I am not a financial adviser. It shows what those reports imply if taken at face value, with sizes you can change.

## Basis: sources, scenarios and rules

The portfolio translates two documents into positions: the [statistical report on the 10 November Trends Journal issue](https://claude.ai/artifact/KM9asSGNh4azHKkVtnDDyq) and the forecast report built from it ("Trend Forecast Report: Markets, Macro, Fiscal, Technology and Military Outlook After the 3 November 2026 Midterms"). The forecast report supplies the scenarios, price targets and probabilities. Live quotes were checked on 10-11 November.

**Window.** Positions open on 11 November 2026 at the 10 November closes and are valued on 11 March 2027. Listed options use the 19 March 2027 expiry.

**Scenario weights.** The forecast report gives bull 25%, base 50% and bear 25% through end-2027, with an Iran re-escalation (about 20% by mid-2027) that can overlay base or bear. For a four-month window I use bull 25%, base 45%, bear 20% and re-escalation 10%.

**Four-month targets.** Index, gold and Brent values sit about 40% of the way between the forecast report's end-2026 and mid-2027 figures. Yield changes outside the base case, and the bear and re-escalation values for gold, are my assumptions.

| Series | Entry, 10 Nov | Bull | Base | Bear | Re-escalation |
| --- | --- | --- | --- | --- | --- |
| S&P 500 | 7,521 | 7,965 (+5.9%) | 7,105 (-5.5%) | 6,100 (-18.9%) | 6,655 (-11.5%) |
| Nasdaq Composite | 25,066 | 27,300 (+9%) | 23,400 (-7%) | 19,700 (-21%) | not modeled |
| Semiconductors (SOX, entry = 100) | 100 | 112 | 87 | 71 | 82 |
| Fed funds upper bound | 4.00% | 4.25% | 4.25%, one more hike by June | 4.25% | 4.50% |
| 2-year Treasury yield | about 4.6% | -28 bp | -10 bp | -48 bp | +27 bp |
| 10-year Treasury yield | about 4.95% | -35 bp | -16 bp | -40 bp | +30 bp |
| Gold ($/oz) | 4,033 | 4,050 | 4,185 | 4,395 | 4,435 |
| Brent ($/bbl) | 91.49 | 69 | 82 | 78 | 119 |

**Simulation rules.**

- No leverage and no short stock. Bearish exposure comes only from bought option spreads, so the most they can lose is the premium.
- Option premium is capped at 2.4% of capital and Kalshi stakes at 2% ($1M) or 1% ($10M).
- Every target weight is the same at both sizes except Kalshi and T-bills. Instruments differ (Section "$1M versus $10M").
- Instruments named are representative, liquid examples for each exposure. Equivalent funds or baskets would serve.
- Positions are rebalanced only on the calendar dates and triggers in "Risk rules and calendar."

## Allocation by sleeve

Half the capital earns Treasury carry. The other half takes the positions the two reports favor, and a small premium budget pays for hedges.

&#91;embedded content: Target weights set in this simulation on 11 Nov 2026; where two figures are shown they are $1M / $10M\]

| Sleeve | $1M | $10M | Holds |
| --- | --- | --- | --- |
| Sovereign debt | $516,000 | $5,260,000 | T-bills, 2-5 and 7-10 year notes, short TIPS |
| Equities | $200,000 | $2,000,000 | Defense, health care, staples, energy, equal-weight S&P 500, gold miners |
| Commodities | $110,000 | $1,100,000 | Gold, silver, uranium, rare earths |
| Real estate | $80,000 | $800,000 | Agency mortgage-backed securities; data-center, health-care and net-lease REITs |
| Bonds (credit) | $50,000 | $500,000 | Short investment-grade corporates |
| Options (premium) | $24,000 | $240,000 | Put spreads on the S&P 500, semiconductors and high yield; a Brent call spread |
| Prediction markets | $20,000 | $100,000 | Kalshi: two Fed-meeting contracts and one shutdown contract |
| **Total** | **$1,000,000** | **$10,000,000** |  |

**What is left out on purpose.** The portfolio holds no semiconductors, AI cloud providers or memory makers, no high-yield or private credit, no office or apartment property, no outright long oil and no Bitcoin. The forecast report's base case has each of these falling or being marked down by mid-2027. Treasuries beyond 10 years are also left out, because of heavy supply and the $41.1 trillion debt limit expected to bind in early 2027.

## Prediction markets (Kalshi)

The sleeve holds three contracts where the forecast report's probabilities sit at least five points away from Kalshi's prices. Two of the three express a single view: the Fed hikes less in the first quarter than Kalshi implies.

**Live prices.** On the evening of 10 November, [Kalshi's Fed page](https://kalshi.com/fed-rate-odds) showed an 86% chance of a 25 bp hike on 9 December and 12% for a larger one. For 27 January it showed hold 51% and a 25 bp hike 48%. For 17 March it showed hold 41% and a hike 56%. Each meeting is its own market.

| Contract | Side | Last quote | Entry limit (modeled fill) | My probability | $1M stake | $10M stake | Resolves |
| --- | --- | --- | --- | --- | --- | --- | --- |
| FOMC 27 Jan 2027: Hold | Yes | 51¢ | 55¢ (53¢) | 68% | $10,000 (about 18,300 contracts) | $55,000 (about 100,500) | 27 Jan 2027 |
| FOMC 17 Mar 2027: Hold | Yes | 41¢ | 46¢ (43¢) | 64% | $6,000 (about 13,400) | $30,000 (about 67,100) | 17 Mar 2027 |
| Government shutdown on 12 Dec 2026 | No | 68¢ on 20 Oct | 72¢ (70¢) | 77% | $4,000 (about 5,600) | $15,000 (about 21,000) | 12 Dec 2026 |

**Why the Fed contracts.** The forecast report has the Fed hiking on 9 December and "once more in H1 2027," with a 60% chance that rates peak at 4.25-4.50%. That leaves one further hike spread across four meetings to June. Kalshi's 48% for January and 56% for March together price about one further hike by mid-March. The 68% and 64% hold probabilities are my derivation from the report's path. They range from 90% in the bull case to 40-45% if the war re-escalates and oil lifts inflation.

**Why the shutdown contract.** The forecast report puts a shutdown of at least one day between December and February at 35%. I assign about two-thirds of that risk to the 11 December deadline, or roughly 23%. The [last quote found](https://next.io/prediction-markets/trending/will-there-be-a-government-shutdown-on-december-12-2026/) was 34¢ for Yes on 20 October, before the election. The contract pays only if part of the government is shut at 10 a.m. on 12 December.

**Expected value and worst case.** After taker fees, the sleeve's expected gain is about $5,400 on $20,000 at $1M and $27,800 on $100,000 at $10M. Each contract pays $1 or nothing, so the whole stake can be lost. The two Fed contracts would likely lose together: Governor Waller has floated a 50 bp move, and another hot CPI print would hurt both.

**Checked and not held**

| Contract | Kalshi price | Forecast report | Verdict |
| --- | --- | --- | --- |
| 9 Dec: hike of 25 bp | 86¢ | 85% | No edge |
| Fed funds above 4.00% after December | 96¢ | About 99% for any hike (CME, 11 Nov) | Too little left after fees |
| US-Iran nuclear deal before 1 Mar 2027 | Yes 15¢ ([CoinStats](https://coinstats.app/prediction-markets/odds/politics/kalshi-kxusairanagreement-27/)) | 40% by mid-2027 for a broader "framework or ceasefire" | No clear edge; definitions differ |
| US-Iran nuclear deal before 1 Jul 2027 | Yes 24¢, about $200 traded a day | Same 40%, broader event | Watch only. It would hedge the oil and gold positions but is too thin |
| S&P 500 year-end range | No current quote found | 7,250 at end-2026 | Expressed through the put spread instead |

**Execution rules.**

- Use resting limit orders. Kalshi's fee is the contract count times price times (1 - price), at 1.75% for resting orders and 7% for orders that take liquidity ([River Markets](https://www.rivermarkets.com/insights/kalshi-fees.html)).
- Do not pay above the limit. Any unfilled stake stays in T-bills.
- The shutdown market is thin, with about $15,000 traded in total by 20 October. The $10M stake may fill only in part.
- Read each contract's rules and position limits on Kalshi before sizing.
- The March contract resolves six days after the window closes. It is valued on 11 March at its conditional probability.

## Equities (U.S. exchanges)

The 20% equity sleeve holds sectors the reports expect to hold up while the index falls, and it avoids the AI complex entirely. With the option hedges, net equity exposure starts close to zero and turns short as prices fall.

| Position | Weight | $1M | $10M | Representative instruments | Thesis from the reports | Modeled return: base / bear |
| --- | --- | --- | --- | --- | --- | --- |
| Defense and aerospace | 5.0% | $50,000 | $500,000 | ITA at $1M; a basket of LMT, RTX, NOC, GD, LHX at $10M | Interceptor shortages and a FY2027 defense topline near $1.15T against $901B now. The sector fell 3-7% on 4 November on fears of Democratic scrutiny ([Reuters](https://whbl.com/?p=1024205)) | +3% / -6% |
| Health care | 3.5% | $35,000 | $350,000 | XLV | Defensive earnings; low sensitivity to AI capital spending | -2% / -9% |
| Consumer staples / low volatility | 3.0% | $30,000 | $300,000 | XLP or USMV | Defensive ballast | -2% / -9% |
| Energy: LNG, midstream, refiners | 3.0% | $30,000 | $300,000 | Cheniere (LNG), a midstream fund, VLO, PSX | Crude flows have largely recovered but LNG and refined products stay tight. Gains most if the war re-escalates | -4% / -14% |
| Equal-weight S&P 500 | 3.5% | $35,000 | $350,000 | RSP | Keeps some upside in a rally without megacap and chip concentration | -4.5% / -17% |
| Gold miners | 2.0% | $20,000 | $200,000 | GDX | Leverage to the forecast report's rising gold path | +5% / +4% |

**Why no technology.** The forecast report gives 60% odds that the Nasdaq makes a lower low under 24,000 in the first half of 2027. It expects chip earnings revisions to turn negative, and puts a 55% chance on a public markdown of GPU-backed loans by end-2027. Nvidia alone was 7.6% of the Nasdaq-100 fund QQQ at mid-year ([24/7 Wall St.](https://247wallst.com/investing/2026/11/06/tech-stocks-just-had-their-worst-day-since-2020/)).

**What would change this sleeve.** Two events inside the window test the thesis. Nvidia is expected to report on 17 November, and the large cloud companies give 2027 capital-spending guidance in late January and February. If they guide at or above the $935B consensus, the forecast report's below-consensus call fails and the chip hedge should close.

**Main risks.** Defense could keep falling if Congress cuts named programs. TD Cowen told Reuters that Trump-branded projects such as Golden Dome are the most exposed. Energy shares lose in a peace deal. Defensive sectors would lag badly in a rally.

## Options overlay

Four bought spreads cost 2.4% of capital and supply most of the gain when markets fall. Each is a spread, so the premium is the most it can lose. All use the 19 March 2027 expiry except Brent, which uses options on the April 2027 future.

| Structure | Premium budget | $1M | $10M | Strikes | Model price | Notional, % of capital | Modeled P&L, % of capital: bull / base / bear / re-escalation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| S&P 500 put spread, long 100% / short 85% of spot | 1.0% | 4 SPY spreads, about $9,900 | 4 SPX spreads, about $99,700 | SPY 750 / 640; SPX 7,520 / 6,400 | 3.3% of notional | 30% | -1.0 / +0.7 / +3.5 / +2.5 |
| Semiconductor put spread (SMH), long 100% / short 75% | 0.8% | 2-3 spreads at about $3,400 each | 23 spreads, about $79,000 | About 465 / 350 | 7.4% of notional | 11% | -0.8 / +0.6 / +1.9 / +1.2 |
| Brent call spread, $100 / $125 | 0.4% | Call spreads on the Brent fund BNO, about $4,000 | 14 futures-option spreads, about $41,000 | 100 / 125 with the April future near $87 | $2.93 a barrel | 14% | -0.4 / -0.4 / -0.4 / +1.2 |
| High-yield bond put spread (HYG), long 97% / short 88% | 0.2% | About 23 spreads, $2,000 | About 229 spreads, $20,000 | 97% / 88% of spot | 1.1% of notional | 18% | -0.2 / -0.2 / +0.7 / +0.1 |
| **Total** | **2.4%** | **$24,000** | **$240,000** |  |  |  | **-2.4 / +0.7 / +5.7 / +4.9** |

**What each one is for.**

- **S&P 500 put spread.** It starts paying as soon as the index falls, so it earns in the base case (-5.5%) and pays its 15-point maximum in the bear case.
- **Semiconductor put spread.** This is the direct expression of the reports' AI thesis. The forecast report has chips down about 13% by March in the base case and 29% in the bear case.
- **Brent call spread.** It pays only if the war re-escalates and Brent returns above $100. The forecast report puts that at about 20% by mid-2027, with Brent near $119 by March on that path.
- **High-yield put spread.** It covers a credit event. The forecast report expects loans backed by AI chips to be repriced in 2027.

**Pricing caveats.** The prices are Black-Scholes estimates on assumed implied volatilities (Appendix C), and no live option quotes were available. Real quotes may cost more. The VIX was 25 in the week to 6 November and put skew had risen sharply ([Cboe](https://www.cboe.com/insights/posts/week-of-11-9-2026-no-contagion-yet-equity-vol-spikes-macro-vol-fades)). SMH closed at $445.91 on 5 November ([The Trading Tools](https://www.thetrading.tools/chart-of-the-day/2026-11-05-smh-second-worst-day)); the $465 entry assumes part of the later rebound. Contract counts are rounded, and the scenario model uses the budgets.

**Rules.** If a spread costs more than modeled, buy less notional and keep the budget. Sell half of any spread that reaches 70% of its maximum value. Sell no uncovered options.

## Sovereign debt and bonds

Treasuries and short investment-grade credit make up 57% of the portfolio and are kept short: the whole portfolio's interest-rate duration is about 1.4 years. Yields near 4.6-5% pay about 1.5% over four months while the Fed is still hiking.

**Reference yields.** On 3 November Treasuries settled at 4.616% (2-year), 4.743% (5-year), 4.950% (10-year) and 5.320% (30-year), with the overnight rate SOFR at 3.95% ([Newsquawk](https://www.newsquawk.com/headlines/treasury-wrap-t-note-futures-z6-settle-11-ticks-lower-at-106-16)). Yields dipped after the election and, per the forecast report, the 10-year moved back toward 5% after the 10 November CPI release.

| Position | Weight | $1M | $10M | Implementation | Role | Modeled return: bull / base / bear / re-escalation |
| --- | --- | --- | --- | --- | --- | --- |
| T-bills, 0-6 months | 25.6% ($1M); 26.6% ($10M) | $256,000 | $2,660,000 | Bill fund such as SGOV at $1M; a ladder of 4- to 26-week bills at $10M | Liquidity for rebalancing; reprices upward after the 9 December hike | +1.35% in all; +1.4% on re-escalation |
| Treasury notes, 2-5 years | 14% | $140,000 | $1,400,000 | SHY and IEI, or 2-, 3- and 5-year notes | Carry of about 4.7%; gains if the Fed hikes less than priced | +2.3 / +1.8 / +2.9 / +0.8 |
| Treasury notes, 7-10 years | 6% | $60,000 | $600,000 | IEF, or 7- and 10-year notes | Flight-to-quality hedge for the bear case | +4.2 / +2.8 / +4.6 / -0.6 |
| TIPS, 0-5 years | 6% | $60,000 | $600,000 | STIP or VTIP, or individual TIPS | Inflation accrual while CPI runs near 4% | +1.6 / +1.9 / +1.8 / +2.6 |
| Short investment-grade corporates, 1-5 years | 5% | $50,000 | $500,000 | VCSH or IGSB at $1M; individual bonds that avoid AI-linked issuers at $10M | Extra yield with little spread duration | +2.7 / +1.5 / +0.9 / +0.1 |

**Why so short.** Three things in the forecast report argue against long bonds. The Fed is expected to hike again. The $41.1 trillion debt limit is expected to bind in the first quarter. And the bear case is two-sided for the 10-year yield, ending 2027 at either 4.0% or 5.4%. The 7-10 year position is sized so that a 45 bp rise costs the portfolio about 0.1%.

**Why little credit.** In September, AI-related investment-grade issuers traded near 115 bp over Treasuries against 78 bp for the broad market, and hyperscaler debt issuance was forecast at a record $420 billion for 2027 ([Bloomberg via BNN](https://www.bnnbloomberg.ca/investing/2026/09/22/corporate-bond-buyers-get-picky-with-flood-of-ai-debt/)). The portfolio therefore holds no high yield, no private-credit vehicles and no long corporates. A fund such as VCSH still includes technology issuers, which is why the $10M version buys individual bonds.

**One shared bet.** The 2-5 year notes and the Kalshi Fed contracts both gain if the Fed hikes less than markets price. They are a single view held in two places.

## Commodities and strategic resources

The 11% commodity sleeve is mostly gold, with small positions in silver, uranium and rare earths. Oil is held only as a call spread, because the forecast report's base case has Brent falling about 10% by March.

**Reference prices.** Indicative prices early on 9 November were gold $4,044.55 an ounce, silver $52.39, copper $6.29 a pound and uranium $91.44 a pound ([Skillings](https://www.skillings.net/assay/2026-11-09/)). The forecast report has gold near $4,033 on 10 November after a 2.6% fall on the CPI release.

| Position | Weight | $1M | $10M | Representative instruments | Thesis from the reports | Modeled return: bull / base / bear / re-escalation |
| --- | --- | --- | --- | --- | --- | --- |
| Gold | 7.0% | $70,000 | $700,000 | GLD or IAU at $1M; a fund plus allocated bullion or futures at $10M | The forecast report has gold at $4,100 by year-end and $4,300 by mid-2027. It now trades about 28% below January's record | +0.4% / +3.8% / +9% / +10% |
| Silver | 1.5% | $15,000 | $150,000 | SLV or PSLV | Follows gold with more volatility; industrial demand hurts it in a crash | +4% / +3% / -4% / +6% |
| Uranium | 1.5% | $15,000 | $150,000 | URNM or the Sprott Physical Uranium Trust | Grid power, not chips, is the binding constraint on AI in the forecast report's base case | +8% / +2% / -12% / +5% |
| Rare earths and critical minerals | 1.0% | $10,000 | $100,000 | REMX, or a basket of producers | Defense supply chains: the forecast report expects interceptor and counter-drone production to get budget priority | +5% / 0% / -12% / +6% |
| Brent crude | Premium only | See Options | See Options | $100 / $125 call spread | Re-escalation insurance | See Options |

**Why gold dominates.** Gold gains in every scenario modeled, and most in the two that are worst for equities. The Trends Journal expects precious metals to dip while rates rise and then spike when equities break. The forecast report rates that call medium, and puts 55% on gold above $4,400 by end-2027. Higher real yields are the main risk: gold fell 2.6% on 10 November on a hawkish repricing.

**Why no outright oil or copper.** Brent has fallen from about $130 in the spring to $91.49, and the forecast report expects $78 by mid-2027 unless the war re-escalates. Copper is near record levels and tied to the AI buildout the portfolio is hedged against.

**Uranium and rare earths are the riskiest lines.** Both would fall hard in the bear case, which is why together they are 2.5% of capital.

## Real estate

The 8% real-estate sleeve is half government-backed mortgage bonds and half listed REITs in three niches. It holds no office or apartment exposure and no direct property, which cannot be bought and sold inside four months.

**Market backdrop.** The 30-year mortgage rate averaged 7.18% on 5 November against 6.22% a year earlier. Purchase applications were down 19% year on year, and Redfin's pending sales were the lowest in records back to 2021 ([Real Estate News](https://realestatenews.com/2026/11/05/mortgage-rates-fall-again-but-buyer-activity-stays-muted)).

| Position | Weight | $1M | $10M | Representative instruments | Thesis from the reports | Modeled return: bull / base / bear / re-escalation |
| --- | --- | --- | --- | --- | --- | --- |
| Agency mortgage-backed securities | 4.0% | $40,000 | $400,000 | MBB or VMBS at $1M; a managed account of agency pools at $10M | Mortgage rates above 7% give government-backed bonds a high yield with no credit risk | +3.3% / +2.6% / +2.2% / -1.0% |
| Data-center REITs (incumbents) | 1.5% | $15,000 | $150,000 | EQIX, DLR | Moratoria make existing capacity scarcer | +5% / -2% / -12% / -5% |
| Health-care REITs | 1.5% | $15,000 | $150,000 | WELL, VTR | Senior-housing demand that does not depend on the economic cycle | +4% / 0% / -8% / -4% |
| Net-lease REITs | 1.0% | $10,000 | $100,000 | O, NNN | Long leases; the most rate-sensitive of the three | +5% / +1% / -7% / -5% |

**The data-center logic cuts both ways.** Voters approved 40 of 41 restrictions on new data centers on 3 November, and New York and Texas had already paused new projects. That hurts developers but raises the value of capacity already built in big cities. Data-center REITs had returned 36% for the year by late August, so they are not cheap, and the position is small ([Weiss Ratings](https://weissratings.com/en/weiss-ratings-daily/the-data-center-freeze-that-s-boosting-reits)). They would still fall with the market in the bear case.

**What is avoided.** The forecast report keeps apartment loan delinquencies above 6.5% through 2027 with the 10-year yield near 5%, and rates continued stress in commercial property as highly likely. Office, apartment owners, mortgage REITs and private property funds are therefore excluded.

**Direct property at $10M.** Distressed commercial debt may become attractive later in 2027 if the forecast report is right. The T-bill sleeve is the reserve for that, but no purchase is modeled inside this window.

## Scenario P&L

The portfolio is modeled to gain in all four scenarios, but for different reasons in each, and the rally case depends on the Kalshi contracts paying.

&#91;embedded content: This simulation's scenario model · 4 months to 11 Mar 2027 · price paths from the forecast report, sector and option assumptions in Appendix C\]

**Base case.** Treasury carry, the two equity put spreads and gold do the work. The equity longs are roughly flat because defense and gold miners offset losses elsewhere.

**Bear and re-escalation.** The hedges add about five to six points. They more than cover a 1.8-point equity loss in the bear case. On re-escalation, defense, energy shares and gold also gain.

**Bull.** The hedges cost their full 2.4 points. The total stays positive only because the Fed contracts are likely to pay in a soft landing, and that is a probability, not a certainty. Without the Kalshi sleeve the four totals are -0.4%, +1.9%, +5.1% and +7.0%.

**Stress case.** The worst combination is a rally in which yields also rise 45 bp and gold falls 5%. That loses about 2.6% before Kalshi. If every Kalshi contract also loses, the loss is 4.6% at $1M and 3.6% at $10M.

**Limits of the model.** Each scenario is one path, not a range. Option values are taken at expiry, and the Kalshi sleeve is shown at its probability-weighted value. Real outcomes will mix the scenarios.

## Risk rules and calendar

The simulation follows fixed rules, so its result can be checked on 11 March against what was written on 11 November.

**Loss limits.**

- Options can lose at most their premium, 2.4% of capital. Kalshi can lose at most its stake, 2% or 1%.
- No single equity, REIT or commodity position exceeds 7% of capital. Only gold is that large.
- If the portfolio is down 4% from its starting value, the Kalshi sleeve is closed and equity longs are halved into T-bills.

**Triggers that change positions.** The first four come from "falsified if" tests in the forecast report.

| If this happens | The report's call that fails | Portfolio response |
| --- | --- | --- |
| The Nasdaq makes a new high before making a lower low | Nasdaq under 24,000 in H1 2027 (60%) | Halve both equity put spreads; raise equal-weight S&P 500 to 7% |
| Large cloud companies guide 2027 capital spending at or above $935B | 2027 capex below consensus (60%) | Close the semiconductor put spread |
| The Fed holds or hikes 50 bp on 9 December | 25 bp hike (85%) | Re-price the January and March Kalshi contracts; cut to half if the gap is under 10 points |
| Brent closes above $105 | Brent under $80 by mid-2027 (55%) | Sell half the call spread; add 2% to TIPS from bills |
| A US-Iran framework is announced | Frozen conflict | Close the Brent call spread; halve energy shares |
| Any spread reaches 70% of its maximum value | - | Sell half; keep the rest to expiry |

**Calendar inside the window**

| Date | Event | What it tests |
| --- | --- | --- |
| 17 Nov 2026 | Nvidia earnings (expected, not confirmed) | Semiconductor hedge |
| 1 Dec | Eurozone flash inflation | Global rate path |
| 8-9 Dec | Federal Reserve meeting | 25 bp hike expected; Fed contracts re-priced |
| 11-12 Dec | Federal funding deadline; Kalshi shutdown contract resolves | Shutdown contract; bill yields |
| 17 Dec | ECB meeting | Futures price about 82% odds of a hike |
| 3 Jan 2027 | New Congress sworn in | Data-center, war-powers and budget bills begin |
| 27 Jan | Federal Reserve meeting | January Hold contract resolves |
| Late Jan-Feb | Cloud companies' 2027 capital-spending guidance | Core AI thesis |
| 1 Feb | Treasury refunding announcement | Supply pressure on 7-10 year notes |
| First quarter | $41.1T debt limit expected to bind | Bill ladder should avoid maturities near any projected default date |
| 11 Mar | Simulation valued | - |
| 17 Mar | Federal Reserve meeting | March Hold contract resolves |
| 19 Mar | Listed options expire | - |

**Rebalancing.** Weights are reset to target on 15 December and 1 February, and after any trigger above. Cash from resolved Kalshi contracts and sold spreads goes to T-bills unless a rule sends it elsewhere.

## $1M versus $10M

The two portfolios hold the same exposures at the same weights. They differ in the instruments used and in how much Kalshi can absorb.

| Area | $1M | $10M |
| --- | --- | --- |
| Kalshi sleeve | 2% ($20,000); fills easily in the Fed markets | 1% ($100,000); worked over several days with resting orders. The shutdown market may fill only in part |
| Treasuries | Funds (SGOV, SHY, IEI, IEF, STIP) for simplicity | Individual bills, notes and TIPS bought at auction or from a dealer, laddered by maturity |
| Credit | A short corporate fund, accepting some technology issuers | Individual bonds, which allows AI-linked issuers to be excluded |
| Equities | Sector funds | Stock baskets within each sector, with funds where a basket adds nothing |
| Index options | SPY options: one contract covers about $75,000 | SPX options: one contract covers about $752,000 and settles in cash |
| Oil options | Options on the Brent fund BNO | Options on Brent futures, which need a futures account |
| Gold | A fund | A fund plus allocated bullion or futures, to spread custody |
| Agency mortgage bonds | A fund | A managed account of specified pools |
| Rounding error | High: two or three SMH spreads cannot match the budget exactly | Low |
| Liquidity | Every position can be closed in a day | Kalshi stakes, single bonds and the thinner REITs may take several days |

**Costs not modeled.** Fund fees, commissions and bid-ask spreads are left out. At $1M the option spreads are where bid-ask costs matter most, because each spread is small. Tax treatment also differs by instrument and investor, and is outside this simulation.

## Appendix A: Full position ledger

Every position at both sizes. Weights are identical except for the Kalshi contracts and T-bills. Where two instruments are shown, the first is the $1M implementation and the second the $10M one.

| Sleeve | Position | Instruments ($1M / $10M) | Weight $1M | $1M | Weight $10M | $10M |
| --- | --- | --- | --- | --- | --- | --- |
| Prediction markets | Kalshi: FOMC 27 Jan 2027, Hold (Yes) | Kalshi contract, limit 55¢ | 1% | $10,000 | 0.55% | $55,000 |
| Prediction markets | Kalshi: FOMC 17 Mar 2027, Hold (Yes) | Kalshi contract, limit 46¢ | 0.6% | $6,000 | 0.3% | $30,000 |
| Prediction markets | Kalshi: shutdown on 12 Dec 2026 (No) | Kalshi contract, limit 72¢ | 0.4% | $4,000 | 0.15% | $15,000 |
| Sovereign debt | T-bills, 0-6 months | SGOV / bill ladder | 25.6% | $256,000 | 26.6% | $2,660,000 |
| Sovereign debt | Treasury notes, 2-5 years | SHY, IEI / 2-, 3-, 5-year notes | 14% | $140,000 | 14% | $1,400,000 |
| Sovereign debt | Treasury notes, 7-10 years | IEF / 7- and 10-year notes | 6% | $60,000 | 6% | $600,000 |
| Sovereign debt | TIPS, 0-5 years | STIP, VTIP / individual TIPS | 6% | $60,000 | 6% | $600,000 |
| Bonds (credit) | Short investment-grade corporates | VCSH, IGSB / individual bonds | 5% | $50,000 | 5% | $500,000 |
| Real estate | Agency mortgage-backed securities | MBB, VMBS / agency pools | 4% | $40,000 | 4% | $400,000 |
| Real estate | Data-center REITs | EQIX, DLR | 1.5% | $15,000 | 1.5% | $150,000 |
| Real estate | Health-care REITs | WELL, VTR | 1.5% | $15,000 | 1.5% | $150,000 |
| Real estate | Net-lease REITs | O, NNN | 1% | $10,000 | 1% | $100,000 |
| Equities | Defense and aerospace | ITA / LMT, RTX, NOC, GD, LHX | 5% | $50,000 | 5% | $500,000 |
| Equities | Health care | XLV | 3.5% | $35,000 | 3.5% | $350,000 |
| Equities | Consumer staples / low volatility | XLP, USMV | 3% | $30,000 | 3% | $300,000 |
| Equities | Energy: LNG, midstream, refiners | LNG, midstream fund, VLO, PSX | 3% | $30,000 | 3% | $300,000 |
| Equities | Equal-weight S&P 500 | RSP | 3.5% | $35,000 | 3.5% | $350,000 |
| Equities | Gold miners | GDX | 2% | $20,000 | 2% | $200,000 |
| Commodities | Gold | GLD, IAU / bullion, futures | 7% | $70,000 | 7% | $700,000 |
| Commodities | Silver | SLV, PSLV | 1.5% | $15,000 | 1.5% | $150,000 |
| Commodities | Uranium | URNM, Sprott Physical Uranium | 1.5% | $15,000 | 1.5% | $150,000 |
| Commodities | Rare earths and critical minerals | REMX / producer basket | 1% | $10,000 | 1% | $100,000 |
| Options | S&P 500 put spread, 100% / 85%, 19 Mar 2027 | SPY / SPX options | 1% | $10,000 | 1% | $100,000 |
| Options | SMH put spread, 100% / 75%, 19 Mar 2027 | SMH options | 0.8% | $8,000 | 0.8% | $80,000 |
| Options | Brent call spread, $100 / $125 | BNO options / Brent futures options | 0.4% | $4,000 | 0.4% | $40,000 |
| Options | HYG put spread, 97% / 88%, 19 Mar 2027 | HYG options | 0.2% | $2,000 | 0.2% | $20,000 |
| **Total** |  |  | **100%** | **$1,000,000** | **100%** | **$10,000,000** |

## Appendix B: Kalshi and option contract detail

The contract-level arithmetic behind the two sleeves that can lose their whole stake.

**B1. Kalshi contracts** (modeled fill plus Kalshi's 7% taker fee on price times one minus price)

| Contract | Cost per contract | Contracts: $1M / $10M | Profit if right | Loss if wrong | Win probability: bull / base / bear / re-escalation | Weighted probability | Expected gain |
| --- | --- | --- | --- | --- | --- | --- | --- |
| FOMC 27 Jan: Hold (Yes) | 53¢ + 1.74¢ | 18,267 / 100,468 | $8,267 / $45,468 | $10,000 / $55,000 | 90% / 65% / 60% / 45% | 68% | $2,467 / $13,570 |
| FOMC 17 Mar: Hold (Yes) | 43¢ + 1.72¢ | 13,418 / 67,091 | $7,418 / $37,091 | $6,000 / $30,000 | 85% / 60% / 60% / 40% | 64% | $2,621 / $13,106 |
| Shutdown on 12 Dec (No) | 70¢ + 1.47¢ | 5,597 / 20,988 | $1,597 / $5,988 | $4,000 / $15,000 | 77% in all | 77% | $310 / $1,161 |
| **Total** |  |  |  | **$20,000 / $100,000** |  |  | **$5,398 / $27,837** |

Resting orders would cut the fee to a quarter. The win probabilities are my estimates, and the expected gain is very sensitive to them: at 58% instead of 68%, the January contract's expected gain falls to about $600 at $1M.

**B2. Option spreads** (Black-Scholes; 128 days to the 19 March expiry; 4% interest rate)

| Structure | Underlying level assumed | Implied volatility: long strike / short strike | Model premium | Value at expiry, % of underlying: bull / base / bear / re-escalation | Maximum value |
| --- | --- | --- | --- | --- | --- |
| S&P 500 put spread, 100% / 85% | 7,521 (SPY about $750) | 22% / 29.5% | 3.31% | 0 / 5.5 / 15 / 11.5 | 15% |
| SMH put spread, 100% / 75% | About $465 | 44% / 52% | 7.38% | 0 / 13 / 25 / 18 | 25% |
| Brent call spread, $100 / $125 | April 2027 future about $87 | 50% / 58% | $2.93 a barrel | $0 / $0 / $0 / $12 | $25 |
| HYG put spread, 97% / 88% | Spot = 100 | 10.5% / 16% | 1.10% | 0 / 0 / 5 / 1.5 | 9% |

The re-escalation value for Brent assumes the April future trades near $112 when front-month Brent is $119. Dividend yields used are 1.2% for the S&P 500, 0.4% for SMH and 7% for HYG.

## Appendix C: How the scenario returns were built

Only the index, rate, gold and oil paths come from the forecast report. Everything else below is my assumption and is the first thing to change if you disagree with a result.

**C1. Method by asset class**

| Asset class | How the four-month return is built | Inputs assumed |
| --- | --- | --- |
| T-bills | Yield for four months | About 4.05% average yield, rising after the December hike |
| Treasury notes | Carry plus duration times the fall in yield | Durations of 2.8 years (2-5 year) and 7.5 years (7-10 year); yield changes from the Basis table |
| TIPS | Real yield plus inflation accrual | CPI rising 0.3-0.4% a month |
| Short corporates | Carry plus duration times the change in yield and spread | 5.2% yield; 2.7-year duration; spreads -10 / +10 / +80 / +35 bp |
| Agency mortgage bonds | Carry plus duration times yield change, with wider spreads in bear and re-escalation | 5.6% yield; 6-year duration |
| Equity sectors and REITs | Sensitivity to the S&P 500 path plus a sector view taken from the reports | Table C2; REIT returns in the Real estate section |
| Gold and silver | Forecast report path in the base case; my assumptions elsewhere | Gold +9% in bear, +10% on re-escalation |
| Uranium and rare earths | My assumptions | Commodities section |
| Options | Value at expiry minus premium | Appendix B2; the Brent spread uses the futures form of the model |
| Kalshi | Probability times payout minus cost and fee | Appendix B1 |

**C2. Equity returns by scenario, four months**

| Position | Bull | Base | Bear | Re-escalation |
| --- | --- | --- | --- | --- |
| S&P 500 (reference, from the forecast report) | +5.9% | -5.5% | -18.9% | -11.5% |
| Defense and aerospace | +2% | +3% | -6% | +12% |
| Health care | +3% | -2% | -9% | -4% |
| Consumer staples / low volatility | +3% | -2% | -9% | -4% |
| Energy: LNG, midstream, refiners | -6% | -4% | -14% | +15% |
| Equal-weight S&P 500 | +6% | -4.5% | -17% | -10% |
| Gold miners | -2% | +5% | +4% | +16% |

**C3. Stress case.** The stress case in the Summary assumes a rally with rising yields: equal-weight S&P 500 +6%, defense and health care +2%, energy -6%, gold -5%, silver -6%, gold miners -10%, REITs -3% to -6%, 7-10 year notes -1.7%, agency mortgage bonds -1.4%, and all four option spreads expiring worthless.

**Where the model is most fragile.** Three assumptions carry most of the result. Defense is assumed to rise 3% while the index falls 5.5%. The S&P 500 spread is assumed to cost 3.3% of notional when volatility is elevated. And the Fed-hold probabilities are derived, not quoted. If defense simply tracked the index and the spread cost 4.5%, the base case would fall from about +2.3% to about +1.5%.

## Appendix D: Sources, pricing notes and caveats

Entry levels come from three places: the forecast report's verified 10 November closes, pages opened on 10-11 November, and my estimates where neither was available.

**D1. Where each input comes from**

| Input | Value used | Source | Status |
| --- | --- | --- | --- |
| S&P 500, Nasdaq, Dow, Brent, gold, Bitcoin closes | 10 Nov levels | Forecast report (verified against CNBC and others) | Verified by the report |
| Scenario paths and probabilities | Bull / base / bear, re-escalation | Forecast report | The report's own estimates |
| Kalshi Fed-meeting prices | 10 Nov, 10:45 p.m. ET | [Kalshi](https://kalshi.com/fed-rate-odds) | Opened |
| Kalshi US-Iran deal prices | 11 Nov, 06:09 UTC | [CoinStats](https://coinstats.app/prediction-markets/odds/politics/kalshi-kxusairanagreement-27/) | Opened; third-party display of Kalshi data |
| Kalshi shutdown price | 20 Oct | [next.io](https://next.io/prediction-markets/trending/will-there-be-a-government-shutdown-on-december-12-2026/) | Opened; three weeks old |
| Kalshi fees | 7% taker, 1.75% maker | [River Markets](https://www.rivermarkets.com/insights/kalshi-fees.html) | Opened |
| Treasury yields | 3 Nov settlement | [Newsquawk](https://www.newsquawk.com/headlines/treasury-wrap-t-note-futures-z6-settle-11-ticks-lower-at-106-16) | Opened; a week old |
| VIX and skew | Week to 6 Nov | [Cboe](https://www.cboe.com/insights/posts/week-of-11-9-2026-no-contagion-yet-equity-vol-spikes-macro-vol-fades) | Opened |
| QQQ and Nasdaq-100 | 5 Nov close | [24/7 Wall St.](https://247wallst.com/investing/2026/11/06/tech-stocks-just-had-their-worst-day-since-2020/) | Opened |
| SMH | 5 Nov close | [The Trading Tools](https://www.thetrading.tools/chart-of-the-day/2026-11-05-smh-second-worst-day) | Opened; 10 Nov level estimated |
| Metals and uranium | 9 Nov, indicative | [Skillings](https://www.skillings.net/assay/2026-11-09/) | Opened |
| Mortgage rates and housing | 5 Nov | [Real Estate News](https://realestatenews.com/2026/11/05/mortgage-rates-fall-again-but-buyer-activity-stays-muted) | Opened |
| Defense shares and budget | 4 Nov | [Reuters via WHBL](https://whbl.com/?p=1024205) | Opened |
| Credit spreads | September | [Bloomberg via BNN](https://www.bnnbloomberg.ca/investing/2026/09/22/corporate-bond-buyers-get-picky-with-flood-of-ai-debt/) | Opened; two months old |
| Data-center and health-care REITs | Late August | [Weiss Ratings](https://weissratings.com/en/weiss-ratings-daily/the-data-center-freeze-that-s-boosting-reits) | Opened; dated |
| Option prices, sector returns, Fed-hold probabilities | - | This simulation | My assumptions |

**D2. Caveats**

- **The chain of evidence is long.** The portfolio rests on a forecast report, which rests on a single newsletter issue. The forecast report could not verify about a third of that issue's figures.
- **No live quotes for most instruments.** Fund, stock and option prices were not available, so sizes are in dollars and option prices are modeled. Contract counts are indicative.
- **Some quotes are stale.** The shutdown price is from 20 October, credit spreads from September and REIT data from August.
- **Four months is short.** Several of the forecast report's calls run to mid-2027 or later. A view can be right and still lose money inside this window.
- **The 10 November Treasury curve is assumed.** Yields were taken from 3 November and the report's remark that the 10-year was back near 5%.
- **Probabilities are judgments.** The scenario weights and Kalshi probabilities are subjective. Changing them changes the expected return, not just its precision.

Every assumption above can be changed and the scenario model rerun at either size.
