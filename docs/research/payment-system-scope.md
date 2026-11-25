# What Nepali developers need beyond a gateway SDK

Research date: 2026-11-25. Star counts and release dates are from the GitHub, npm and Packagist APIs on that date.

## Summary

1. Nepal has many thin gateway wrappers and no open-source, self-hosted "payment system" toolkit. The nearest things are a Laravel package with optional payment tables, an Odoo addon with a reconcile cron, and one hosted commercial orchestrator (PayBridgeNP).
2. eSewa, Khalti, ConnectIPS and Fonepay web checkout document only browser redirects. None documents a server-to-server webhook. All four document a status lookup, so polling is the only reliable confirmation path.
3. The most repeated pain point is the payment that succeeds after the customer closed the tab. The fix is always the same three parts: a stored payment attempt, a status lookup, and a job that polls unfinished attempts.
4. A refund API is documented only for Khalti (and for NepalPay QR, but for acquirers, not merchants). eSewa and Fonepay document refunds through the dashboard or app.
5. NepalQR is an EMVCo merchant-presented standard published by NRB. Only banks and licensed PSPs can acquire merchants. A library can encode and parse the payload but cannot make a QR payable.
6. NRB licenses two things: PSPs (intermediate payments between customer and merchant) and PSOs (clearing, switch, "Payment Switch/Gateway"). Software that a merchant runs with the merchant's own gateway accounts is not described as either. Holding or re-routing other merchants' funds is.
7. I could not read the Payment and Settlement Act 2075 or the current (2023) Licensing Policy in full. Both are Nepali-only and the NRB server failed. Treat sections 3.4 and 3.5 as incomplete, not as legal advice.
8. Access, not code, is the first barrier that developers report: business registration, bank enrolment, setup fees, and sandboxes that need a merchant account.
9. Eight candidate directions are in section 5, with trade-offs. Five need no storage; the others end the "stateless, stdlib only" identity of the project.
10. Unanswered questions are listed at the end.

## 1. Existing Nepal payment libraries

All sources in this section are the projects' own READMEs, registries or repository metadata.

### 1.1 Multi-provider libraries

| Project | Lang | Providers | Beyond initiate/verify | Activity | Lacks |
|---|---|---|---|---|---|
| [mukezhz/pay-np](https://github.com/mukezhz/pay-np) | Go | eSewa, Khalti, ConnectIPS, Fonepay, Hamro Pay, IME Pay | `Lookup` as a separate call, `Status.Final()`, `ParseWebhook` for Hamro Pay, Fonepay `GenerateQR` and `QRStatus`, eSewa mobile verify. README tells the user to "run a reconciler" but does not ship one. | 4 stars, v0.2.0 on 2026-10-09 | Storage, reconciler, refunds |
| [aryanmalla19/laravel-nepali-payment](https://github.com/aryanmalla19/laravel-nepali-payment) (Packagist `jaap-tech/laravel-nepali-payment`) | PHP/Laravel | eSewa, Khalti, ConnectIPS | Optional migrations and a `PaymentTransaction` model, status scopes, lifecycle events (`PaymentInitiatedEvent`, `PaymentCompletedEvent`, `PaymentFailedEvent`) | 3 stars, pushed 2026-03-10, no release, [39 installs](https://packagist.org/packages/jaap-tech/laravel-nepali-payment) | Reconcile job, refunds, QR, Fonepay |
| [aryanmalla19/nepali-payment-gateway](https://github.com/aryanmalla19/nepali-payment-gateway) (Packagist `kbk/nepali-payment-gateway`) | PHP | eSewa, Khalti, ConnectIPS | None | 10 stars, pushed 2026-02-26 | Everything beyond wrappers |
| [Nutandc/Nepal-Payment-Gateway](https://github.com/Nutandc/Nepal-Payment-Gateway) | PHP/Laravel | eSewa, Khalti, ConnectIPS, optional Stripe | README lists "Idempotency support to prevent duplicate transactions" (TTL setting) | 0 stars, pushed 2026-01-03 | Storage, reconcile, refunds |
| [S4NKALP/django-nepali-payments](https://github.com/S4NKALP/django-nepali-payments) | Python/Django | Khalti, eSewa, Fonepay, ConnectIPS | Fonepay dynamic and static QR, QR status monitor by polling, Fonepay "tax refund", retries with backoff | 1 star, v1.0.0 on 2026-09-02 | Storage, general refunds |
| [sujankim/nepal-pay-spring-boot-starter](https://github.com/sujankim/nepal-pay-spring-boot-starter) | Java/Spring | Khalti, eSewa, ConnectIPS, Fonepay | Blocking and reactive clients, retry with exponential backoff | 2 stars, v1.2.1 on 2026-07-12 | Storage, reconcile, refunds |
| [shoesheill/Nepal.Payments.Gateways](https://github.com/shoesheill/Nepal.Payments.Gateways) | C# | eSewa, Khalti, Fonepay dynamic QR | Fonepay WebSocket payment tracking, events, SSE example | 5 stars, pushed 2026-09-28, no licence file | Storage, ConnectIPS, refunds |
| [mukundaparajuli/nepal-pay](https://github.com/mukundaparajuli/nepal-pay) (npm [`@nepal-pay/*`](https://www.npmjs.com/package/@nepal-pay/core)) | TypeScript | eSewa, Khalti, ConnectIPS, Fonepay | README calls it "stateless" | 1 star, npm 0.1.0 on 2026-10-23 | Everything beyond wrappers |
| [KritanShahi/nepal-payment-gateway](https://github.com/KritanShahi/nepal-payment-gateway) | TypeScript | eSewa, Khalti | Long README section on reconciliation and idempotent fulfilment; the code is left to the user | 1 star, pushed 2026-09-05 | Storage, other providers |
| [sahilkhatiwada/np-payment-sdk](https://github.com/sahilkhatiwada/np-payment-sdk) | TypeScript | Claims Nepal and global gateways (Stripe, Razorpay, ...) | `refund`, `subscribe`, `createInvoice` method names in README | 5 stars, v1.0.3 on 2025-07-17 | Not confirmed that the Nepal refund methods call a real API |
| [ReaperPsych/odoo18-esewa-khalti-connectips-payment](https://github.com/ReaperPsych/odoo18-esewa-khalti-connectips-payment) | Python/Odoo | eSewa, Khalti, ConnectIPS | Uses Odoo's transaction tables, `UNIQUE` constraints against replay, "15-minute scheduled action" that reconciles pending transactions | 1 star, pushed 2026-11-22 | Only usable inside Odoo |
| [CodeDemonOrg/rn-all-nepal-payment](https://github.com/CodeDemonOrg/rn-all-nepal-payment) | React Native | eSewa, Khalti, ConnectIPS, IME Pay | Client UI only | 23 stars, last push 2021-09-14 | Unmaintained, no server side |

npm also lists `nepal-pay` 1.2.0, `@bivekkk/nepay` 0.2.0, `@nepalpay/esewa` and `@nepalpay/khalti` 0.2.5, and `@lacspace/*` 1.1.x, all published in the last three months and all described as SDKs for initiate, lookup and verify ([npm search](https://www.npmjs.com/search?q=nepal%20payment)). I did not read their sources.

### 1.2 Single-provider libraries worth noting

- [remotemerge/esewa-php-sdk](https://github.com/remotemerge/esewa-php-sdk): 17 stars, v4.1.0 on 2026-11-01. Its README states the boundary clearly: "It does not store orders, track fulfilment, or retry status checks; your application handles those."
- [neputertech/khalti](https://github.com/neputertech/khalti): Laravel wrapper, 14 stars, v1.1.2 on 2025-06-18.
- Most-installed PHP packages are single-gateway: `dipesh79/laravel-esewa` (6,478 installs), `snlbaral/khalti-api` (4,630), `sudiptpa/omnipay-esewa` (4,491) ([Packagist search](https://packagist.org/search/?q=esewa)).
- Official Khalti client SDKs: [web](https://github.com/khalti/khalti-sdk-web) (last push 2023-01), [Android](https://github.com/khalti/khalti-sdk-android) (2024-03), [Flutter](https://github.com/khalti/khalti-flutter-sdk) (2024-05), [iOS](https://github.com/khalti/khalti-sdk-ios) (2024-07), and a [WooCommerce plugin](https://github.com/khalti/woocommerce-plugin-kpg-2) (2023-01). There is no official server SDK in Go, Node or Python in that organisation.
- [prabinpant/fonepay-node](https://github.com/prabinpant/fonepay-node): 10 stars, Fonepay web redirect only.

### 1.3 Things that are not thin wrappers

- [PayBridgeNP](https://paybridgenp.com/) is a hosted commercial service: one API for eSewa, Khalti and Fonepay dynamic QR, hosted checkout page, signed and retried webhooks, idempotency keys, dashboard. Its page says "We never hold your money"; payments settle in the merchant's own gateway accounts. Plans are free, Rs 1,999 and Rs 4,999 per month. The page does not claim an NRB licence and does not say the service is open source. Its SDK is on npm as [`@paybridge-np/sdk`](https://www.npmjs.com/package/@paybridge-np/sdk).
- [RojanSapkota/esewa-payment-gateway](https://github.com/RojanSapkota/esewa-payment-gateway): 27 stars three months after creation (2026-08-24), more than any multi-provider wrapper in 1.1. It is a self-hosted server with SQLite, a QR that encodes a personal eSewa ID, an IMAP listener that matches incoming payment emails to orders by a unique fractional amount, and signed outbound webhooks. It shows demand for "a payment system without a merchant account". It also shows a method this project should not copy (section 5.3).
- [Nepal Payment Solution](https://nepalpayment.com/) advertises an "Aggregated Payment Gateway". Its site did not render for me, so details are not confirmed.

### 1.4 Answer

There is no open-source multi-provider payment-system toolkit for Nepal. The field is thin wrappers. The two partial exceptions are tied to one framework (Laravel tables and events; Odoo cron). The orchestration layer exists only as a hosted commercial product. In Go there is one direct competitor, pay-np, which already has more providers and a separate `Lookup` call than this project has.

## 2. Building blocks in payment infrastructure elsewhere

| Block | Problem it solves | Where it appears |
|---|---|---|
| Payment state machine | One record per payment attempt with a fixed set of states and legal transitions, so "paid" can happen only once and every unfinished payment is visible. | Stripe PaymentIntent states `requires_payment_method`, `requires_confirmation`, `requires_action`, `processing`, `succeeded`, `canceled` ([Stripe](https://docs.stripe.com/payments/paymentintents/lifecycle)). Kill Bill: "the Kill Bill payment subsystem is in charge to enforce the transitions", defined in an XML file ([Kill Bill](https://docs.killbill.io/latest/userguide_payment)). Vendure: `Created`, `Authorized`, `Settled`, `Declined`, plus a custom `PaymentProcess` ([Vendure](https://docs.vendure.io/guides/core-concepts/payment/)). |
| Event log per payment | Amounts authorised, charged and refunded are derived from an append-only list of events, which gives an audit trail and makes partial refunds simple. | Saleor `TransactionItem` with `TransactionEvent`s ([Saleor](https://docs.saleor.io/developer/payments/transactions)). |
| Idempotency keys | A retried request must not create a second payment. The server stores the first result under a client-chosen key. | Stripe stores the status code and body of the first request, compares parameters on reuse, and may prune keys after 24 hours ([Stripe](https://docs.stripe.com/api/idempotent_requests)). Kill Bill uses a `transactionExternalKey` ([Kill Bill](https://docs.killbill.io/latest/userguide_payment)). |
| Webhook ingestion | Confirmation that does not depend on the customer's browser. Needs signature check on the raw body, a timestamp tolerance against replay, duplicate detection by event ID, no reliance on order, and a fast 2xx. | Stripe: HMAC-SHA256 over `timestamp.payload`, 5-minute default tolerance, retries for up to three days, "doesn't guarantee the delivery of events in the order that they're generated" ([Stripe](https://docs.stripe.com/webhooks)). Laravel Cashier ships the webhook controller and signature middleware ([Cashier](https://laravel.com/docs/12.x/billing)). |
| Reconciliation job | A background task that asks the gateway about payments stuck in an unknown or pending state. This replaces webhooks when a gateway has none. | Kill Bill "Janitor": "PENDING and UNKNOWN transaction states should be temporary"; it polls the plugin and corrects the record ([Kill Bill](https://docs.killbill.io/latest/userguide_payment)). Hyperswitch: "Automate 2-way and 3-way reconciliation" ([Hyperswitch](https://github.com/juspay/hyperswitch)). |
| Refunds | A second money movement linked to the first, with its own state, and a refunded total on the payment. | Stripe Refunds API ([Stripe](https://docs.stripe.com/payments/paymentintents/lifecycle)), Saleor `TRANSACTION_REFUND_REQUESTED` ([Saleor](https://docs.saleor.io/developer/payments/transactions)), Medusa "authorize, capture, and refund" ([Medusa](https://docs.medusajs.com/resources/commerce-modules/payment)). |
| Provider plugin interface | The core does not know gateways. Each gateway is an adapter behind one contract. | Kill Bill payment plugins, Medusa payment providers, Vendure `PaymentMethodHandler`, Saleor apps (the core "does not contact payment gateways itself") (same links). This project already has this block. |
| Framework schema and glue | Migrations, a model trait, routes. It removes the work that every application repeats. | Cashier adds columns to `users` and creates subscription tables ([Cashier](https://laravel.com/docs/12.x/billing)). |
| Routing and fallback | Choose a processor per transaction and retry on another. | Hyperswitch "Intelligent Routing" across 120+ processors and retry strategies "tuned by card bin, region, method" ([Hyperswitch](https://github.com/juspay/hyperswitch)). This assumes interchangeable card processors. |
| Vault | Store card data under PCI rules. | Hyperswitch vault ([Hyperswitch](https://github.com/juspay/hyperswitch)). |
| Hosted checkout page | One page that lists methods and handles each redirect or form POST, so the application has one integration. | Stripe Checkout through Cashier ([Cashier](https://laravel.com/docs/12.x/billing)), Hyperswitch drop-in widgets ([Hyperswitch](https://github.com/juspay/hyperswitch)). |
| Ledger | Double-entry record of balances. Needed when the system holds or owes money. | Not a separate module in the projects above. Hyperswitch describes reconciliation and cost observability, not a ledger. A ledger is not confirmed as a common block from these sources. |

Separable for a library: state machine, event log, idempotent "mark paid once", webhook verification helpers, reconciliation loop, refund calls, provider interface. Hyperswitch says "each module is independent", but its README also shows the app server depends on separate vault and encryption services. Not separable without a running service and a database: hosted checkout, routing, vault.

## 3. Nepal-specific facts

### 3.1 Per gateway

| | Notification | Status lookup | Refund API | QR | Settlement | Sandbox and credentials |
|---|---|---|---|---|---|---|
| **eSewa ePay v2** | Browser redirect to `success_url` or `failure_url` with Base64 signed data. No webhook is documented. "If a response is not received within five minutes, the status check API can be used." | Yes: `GET /api/epay/transaction/status/` by `product_code`, `total_amount`, `transaction_uuid`. Statuses `PENDING`, `COMPLETE`, `FULL_REFUND`, `PARTIAL_REFUND`, `AMBIGUOUS`, `NOT_FOUND`, `CANCELED`. | Not documented on the ePay page, although refund statuses exist. | Not on the ePay page. | "merchant can withdraw balance to respective bank account"; merchant wallet at merchant.esewa.com.np. No statement API documented. | Public shared UAT: product code `EPAYTEST`, published secret key, test IDs. Production key "will be provided from eSewa". |
| **Khalti ePayment (KPG-2)** | Redirect to `return_url` with unsigned query parameters. No webhook described; docs "recommend that payment lookup API is checked". | Yes: `POST /epayment/lookup/` by `pidx`. Statuses Completed, Pending, Refunded, Expired, User canceled, Partially Refunded. Link expires in 60 minutes in production. | Yes: `/api/merchant-transaction/{transaction_id}/refund/`, full or partial, wallet and bank. The page says paisa but its example uses `50.00`; HTTP method and auth are not stated. | Not in these docs. | "Amount collected in the Merchant Dashboard can be deposited or transferred to bank accounts at any time." Lookup returns `fee`. No statement API. | Self-service: sign up at test-admin.khalti.com, OTP `987654`, test IDs 9800000000-5. No shared key. |
| **ConnectIPS gateway (NCHL)** | Redirect to a success or failure URL that is registered in advance with NCHL; only `TXNID` is appended. No webhook. | Yes: `validatetxn` and `gettxndetail` (Basic auth plus RSA-signed token). Results `SUCCESS`, `FAILED`, `ERROR` (not found / incomplete). `gettxndetail` returns charge, batch ID and `creditStatus`. | Not documented in the gateway section. | Separate product (NepalPay QR, below). | Not stated in the gateway section. A merchant portal offers "operations, reporting, and reconciliation". | No public sandbox. A merchant must "enroll their application for connectIPS through its bank"; the bank does KYC and NCHL emails the IDs. |
| **Fonepay** | Web: redirect with HMAC-signed parameters (this repository's implementation). QR: a WebSocket and status endpoint are used by community libraries; not confirmed from a Fonepay document. | Not confirmed from a primary source. pay-np's README says web verification needs the `UID` from the redirect, so an abandoned payment cannot be looked up. | Fonepay FAQ describes full and partial refund in the FoneBiz app. API not confirmed. | Static and dynamic QR; "dynamic QR can be integrated directly into your billing system". | Primary source not found. A PayBridgeNP blog (secondary) reports three scheduled payouts a day or paid instant settlement. | No public developer portal. "Merchants can contact their respective bank or Fonepay team for integration." |
| **IME Pay** | `developer.imepay.com.np` now answers with a 301 redirect to docs.khalti.com, and Khalti's docs are copyright "IME Khalti Limited". The remaining public material is a [docs repository](https://github.com/khalti/imepay-dev-docs) with Android, iOS and plugin pages. | Not confirmed (old API pages are gone). | Not confirmed. | Not confirmed. | Not confirmed. | Not confirmed. |
| **NepalPay QR (NCHL)** | WebSocket "QR transaction notification workflow" for QR-parsed and transaction-status events; also a `POST /nepalpay/confirm-payment` call from the QR engine to a "merchant service provider". | Yes: `POST /nQR/v1/merchanttxnreport` by trace ID, or by date range (works as a statement). Final when `debitStatus` is `000` and `creditStatus` is `000`, `DEFER` or `999`. | Yes: `POST /nQR/v1/refund`, "enabled the merchant acquirer to initiate the refund". | `POST /qr/generateQR`, static (`11`) or dynamic (`12`). | `settledsession` and a per-session reconciliation `report` exist for network members. | The stated audience is "BFIs, NCHL members, and service providers". The request fields require an `acquirerId`. A merchant reaches these APIs through its acquirer; direct merchant access is not confirmed. |
| **Hamro Pay Checkout** (new, not in this SDK) | Server-to-server webhook exists: "verify each notification with the separate webhook signing secret". Algorithm not stated on the page (pay-np's README says HMAC-SHA512). | Yes: "Get Transaction". | Not mentioned. | Not mentioned. | Not mentioned. | Self-service UAT signup at pay-sandbox.hamropatro.com. |
| **Card acquiring** | NIC Asia offers a "Cybersource Internet Payment Gateway": needs a service agreement and a NIC Asia current account for settlement; integration fees apply; no public developer docs are linked, support is by email. | Not confirmed. | Not confirmed. | n/a | Timing not stated. | Through the bank. Nabil and Himalayan Bank gateway documents were not found on first-party sites: not confirmed. |

Sources: eSewa [ePay docs](https://developer.esewa.com.np/pages/Epay). Khalti [web checkout](https://docs.khalti.com/khalti-epayment/), [refund](https://docs.khalti.com/api/refund/), [home](https://docs.khalti.com/), [gotchas](https://docs.khalti.com/gotchas/). NCHL [core module](https://doc.connectips.com/docs/connectIPS-Gateway/cips-core-module), [merchant interface](https://doc.connectips.com/docs/connectIPS-Gateway/merchant-interface), [validation](https://doc.connectips.com/docs/connectIPS-Gateway/payment-validation), [transaction details](https://doc.connectips.com/docs/connectIPS-Gateway/txn_details), [status list](https://doc.connectips.com/docs/connectIPS-Gateway/validateTxn-getTxnDetail). NepalPay QR [section index](https://doc.connectips.com/docs/category/-2-nepalpay-qr), [WebSocket](https://doc.connectips.com/docs/NepalPAY-QR/NEPALPAY-QR-Websocket/introduction), [transaction report](https://doc.connectips.com/docs/NepalPAY-QR/Transaction-Report/transaction_report), [API details](https://doc.connectips.com/docs/NepalPAY-QR/NEPALPAY%20API%20Specification/API%20Specification%20Details), [introduction](https://doc.connectips.com/docs/NepalPAY-QR/NEPALPAY%20API%20Specification/introduction). Fonepay [FAQ](https://fonepay.com/faqs), [business page](https://fonepay.com/business), [PayBridgeNP blog, secondary](https://paybridgenp.com/blog/fonepay-api-integration-nepal). Hamro Pay [checkout](https://hamropay.com.np/checkout/), [how it works](http://hamropay.com.np/checkout/developer/how-it-works/). NIC Asia [help article](https://helpdesk.nicasiabank.com/support/solutions/articles/35000164571-cybersource-internet-payment-gateway-ipg-).

### 3.2 What the table means for design

- Polling is not optional. Three of the four supported gateways document no push channel. eSewa's own flow lists the status API as step 6.
- eSewa has two non-final states that a naive integration treats as failure: `PENDING` and `AMBIGUOUS`. `failure_url` is used for "FAILURE or PENDING".
- ConnectIPS callback URLs are fixed per application at NCHL, so a per-request `CallbackURL` cannot work there.
- Khalti's redirect is unsigned and does not carry the merchant's order ID, so the `pidx` must be stored at initiation. This repository's README already tells the user to do that by hand.
- Refund coverage through APIs is one gateway out of four.

### 3.3 NepalQR

- NRB published the "NepalQR Standardization Framework and Guidelines" in February 2021. The specification annex is "Compliant to EMVCo. standard", merchant-presented. The point-of-initiation field is `11` for static and `12` for dynamic ([NRB](https://www.nrb.org.np/psd/nepalqr-standardization-framework-and-guidelines/)).
- Section 4.3: "The merchant acquiring and issuing can be done by Bank & Financial Institution and Payment Service Providers (PSPs) licensed by NRB."
- Section 4.5 mentions "Aggregated means (via third party acquirer) of merchant on-boarding" and requires the end merchant's name and category code to be passed on. The framework therefore knows the aggregator model and places duties on the acquirer.
- Section 4.5 item 12 lists merchant facilities: "SMS/notification for every transaction", "Instant refund facility", dispute tracking. These are duties of the acquirer's merchant app, not an API promise to developers.
- Section 4.7: no charge to the customer; the merchant pays an MDR that the market sets.
- Section 4.14: networks should interoperate "with proper settlement mechanism"; proprietary QRs "shall shift to NepalQR" after a date that NRB defines. The document gives no date. The settlement time to the merchant is not specified.
- NRB's 2023/24 report names three QR networks: Fonepay, "Nepal Pay QR and Smart QR" ([NRB](https://www.nrb.org.np/psd/payment-systems-oversight-report-2080-81-2023-2024/)). The 2024/25 report counts about 2.9 million QR merchants ([NRB](https://www.nrb.org.np/psd/payment-systems-oversight-report-2081-82-2024-2025/)).
- NCHL describes NepalPay QR as "an implementing infrastructure of NepalQR" ([NCHL](https://doc.connectips.com/docs/NepalPAY-QR/NEPALPAY%20API%20Specification/introduction)).
- Whether a Fonepay QR and a NepalPay QR are payable from every app today is not confirmed from a primary source.

### 3.4 NRB rules

What I could read:

- **Licensing Policy 2016 (English).** A PSP is an institution that provides "payment activities through electronic means", including one that will "make payment of goods and service as an intermediary between customers and service provider", and that has a "direct relationship with customers". PSP activity 1 is "Giving/taking payment and intermediate between customer and service provider". A PSO is "established with the objective of operation, control and clearing of payment related activities"; its activity list includes "Payment Switch/Gateway". Minimum paid-up capital in this version: Rs 10 million (PSP by telecom network), Rs 50 million (PSP by card), Rs 100 million (PSO). The words "aggregator" and "merchant" do not appear ([NRB, 2016 policy](https://www.nrb.org.np/ofg/licensing-policy-for-payment-related-institutions-2073-2/)).
- **This policy is superseded.** NRB's 2024/25 report lists "Licensing Policy for Payment Related Institutions (First Amendment, 2024), 2023", "Payment and Settlement Act, 2019", "Payment and Settlement Bylaw (First Amendment, 2023), 2020" and "Payment Systems-Related Unified Directives, 2024" as the current framework ([NRB](https://www.nrb.org.np/psd/payment-systems-oversight-report-2081-82-2024-2025/)). A law firm summary (secondary) says the 2023 policy took effect on 2023-01-02, keeps the two licence types, and raises capital requirements with a deadline of 2028-07-16 ([Pradhan Law](https://pradhanlaw.com/publications/licensing-policy-for-institutions-that-perform-payment-related-work-2079-2023-ad)). Current capital amounts: not confirmed.
- **Definitions in NRB's own report.** "PSP is the institution that provides payment related services to beneficiaries. ... PSO is the institution that performs payment related operations, management, and clearing" ([NRB 2021/22 report, Box 1](https://www.nrb.org.np/psd/payment-systems-oversight-report-2078-79-2021-2022/)).
- **Who holds licences.** At mid-July 2025: 9 PSOs and 22 non-bank PSPs, plus banks and financial institutions licensed as PSPs ([NRB](https://www.nrb.org.np/psd/payment-systems-oversight-report-2081-82-2024-2025/)). Fonepay is "the licensed PSO"; eSewa, Khalti and IME Digital are named as PSPs ([NRB 2023/24](https://www.nrb.org.np/psd/payment-systems-oversight-report-2080-81-2023-2024/)).
- **Regulatory sandbox.** NRB reports "preparatory work towards the operationalization of the Regulatory Sandbox" (same 2024/25 report). It is not open yet according to that text.

What I could not read: the Act itself (posted by NRB only in Nepali, [link](https://www.nrb.org.np/psd/%e0%a4%ad%e0%a5%81%e0%a4%95%e0%a5%8d%e0%a4%a4%e0%a4%be%e0%a4%a8%e0%a5%80-%e0%a4%a4%e0%a4%a5%e0%a4%be-%e0%a4%ab%e0%a4%b8%e0%a5%8d%e0%a4%b0%e0%a5%8d%e0%a4%af%e0%a5%8c%e0%a4%9f-%e0%a4%90%e0%a4%a8/); the download timed out), the 2023 policy text, and the Unified Directives 2024. The section of the Act that prohibits unlicensed operation and its penalty are therefore not confirmed. A news report (secondary) says section 7 requires an application to NRB to operate as a PSO or PSP ([Onlinekhabar](https://english.onlinekhabar.com/open-payment-foreign-companies.html)).

### 3.5 What a library author can and cannot conclude

Can conclude:

- Receiving customers' money and passing it to other merchants is the stated core of a PSP ("intermediate between customer and service provider"). A product that does this needs a licence and capital in the tens of millions of rupees at least. This holds in both the 2016 text and the summaries of the 2023 text.
- Operating a switch or "Payment Switch/Gateway" between institutions is PSO work.
- Acquiring QR merchants is reserved for banks and licensed PSPs. A third party that onboards merchants does so under an acquirer.
- A library or self-hosted server that a merchant runs with the merchant's own credentials, where each gateway settles to the merchant's own account, does not match any activity in the lists that I read. PayBridgeNP publicly operates on this model and makes the "never hold your money" point central.

Cannot conclude:

- That the "own credentials, no funds held" model is exempt. No NRB text that I read says so. It is an absence of a rule, in documents that are partly out of date.
- Whether a multi-tenant SaaS that stores many merchants' gateway keys and initiates payments for them counts as "payment activities through electronic means". The phrase is broad.
- Anything about data-protection, AML or outsourcing duties in the Unified Directives.

The README of any stateful feature should say that the software does not make its operator a licensed entity, and that a platform which collects for third parties must talk to NRB or a licensed PSP.

## 4. Pain points reported by developers (secondary sources)

This section uses GitHub issues, READMEs of community libraries, blogs and Reddit. Reddit pages could not be opened by my tools; for those I had only the titles and search snippets. Counts are the number of distinct sources that I saw, not a survey.

| Theme | About how often | Sources |
|---|---|---|
| Native mobile SDK build breakage (Swift version, Gradle, R8, AndroidX) | About 30 issue titles | [esewa_pnp issues](https://github.com/ayyshim/esewa_pnp/issues?q=is%3Aissue), [khalti-sdk-android issues](https://github.com/khalti/khalti-sdk-android/issues?q=is%3Aissue). Outside the scope of a Go server library. |
| No webhook; payment completes but the application never learns of it | 7 | Khalti docs issue ["Webhook?"](https://github.com/khalti/docs.khalti.com/issues/14) (open since 2019, no reply); WooCommerce plugin ["Payment stays on confirmed state when paid using Mobile banking, Connect IPS, SCT card"](https://github.com/khalti/woocommerce-plugin-kpg-2/issues/17); Android ["No callback on successful cIPS payment"](https://github.com/khalti/khalti-sdk-android/issues/22); READMEs of [pay-np](https://github.com/mukezhz/pay-np), [nepal-payment-gateway](https://github.com/KritanShahi/nepal-payment-gateway) and the [Odoo addon](https://github.com/ReaperPsych/odoo18-esewa-khalti-connectips-payment), which each add a reconcile step; [blog](https://kokil.com.np/blog/laravel-khalti-and-esewa-nepal-payment-integration) ("Duplicate callbacks (common with eSewa)"). |
| Documentation gaps: no OpenAPI file, missing examples, no answer on idempotency, thin Fonepay docs | 7 | Khalti docs issues [#52](https://github.com/khalti/docs.khalti.com/issues/52), [#48](https://github.com/khalti/docs.khalti.com/issues/48), [#59](https://github.com/khalti/docs.khalti.com/issues/59), [#7](https://github.com/khalti/docs.khalti.com/issues/7), [#6](https://github.com/khalti/docs.khalti.com/issues/6), [#12](https://github.com/khalti/docs.khalti.com/issues/12); [PayBridgeNP blog](https://paybridgenp.com/blog/fonepay-api-integration-nepal) (Fonepay covers "the API only in outline"). |
| Calling the gateway from the browser (CORS, "Network Error") | 6 | Khalti docs issues [#74](https://github.com/khalti/docs.khalti.com/issues/74), [#41](https://github.com/khalti/docs.khalti.com/issues/41), [#39](https://github.com/khalti/docs.khalti.com/issues/39), [#19](https://github.com/khalti/docs.khalti.com/issues/19); [khalti-sdk-web #51](https://github.com/khalti/khalti-sdk-web/issues/51); [flutter #20](https://github.com/khalti/khalti-flutter-sdk/issues/20). Khalti's own [gotchas page](https://docs.khalti.com/gotchas/) covers it. |
| Access: business registration, bank enrolment, setup fees, slow approval | 5 | Reddit r/technepal thread titles and snippets: ["Esewa, Khalti, Fonepay Apis Shouldn't cost absurd setup fee"](https://www.reddit.com/r/technepal/comments/1s2n7t4/esewa_khalti_fonepay_apis_shouldnt_cost_absurd/), ["Is it worth it to buy their api for Rs. 25,000? Why Can't I just place the business QR"](https://www.reddit.com/r/technepal/comments/1of1ztk/has_anyone_integrated_esewa_payment_gateway_into/), ["You need a company registration, then a company PAN number"](https://www.reddit.com/r/technepal/comments/1d3ab6i/payment_gatewayskhalti_esewa_integration_for/), [r/devnep](https://www.reddit.com/r/devnep/) ("try getting merchant approval"); [PayBridgeNP blog](https://paybridgenp.com/blog/fonepay-api-integration-nepal) (bank enrolment is where "most delays happen"). |
| Keys and environments: "Invalid token", sandbox key on live, UAT changes | 5 | Khalti docs issues [#71](https://github.com/khalti/docs.khalti.com/issues/71), [#51](https://github.com/khalti/docs.khalti.com/issues/51); [Reddit](https://www.reddit.com/r/technepal/comments/1des1og/khaltipayment_integration/); [neputertech/khalti #10](https://github.com/neputertech/khalti/issues/10); [blog](https://kokil.com.np/blog/laravel-khalti-and-esewa-nepal-payment-integration) ("Khalti occasionally updates sandbox endpoints without notice"). |
| Amount units: paisa or rupees, decimals | 5 | [khalti-sdk-android #30](https://github.com/khalti/khalti-sdk-android/issues/30), [khalti-sdk-ios #3](https://github.com/khalti/khalti-sdk-ios/issues/3), [esewa_pnp #22](https://github.com/ayyshim/esewa_pnp/issues/22), [blog](https://kokil.com.np/blog/laravel-khalti-and-esewa-nepal-payment-integration), and Khalti's [refund page](https://docs.khalti.com/api/refund/), which contradicts itself. This project fixed the unit in v2. |
| Missing merchant-side APIs: refund examples, cancel an issued `pidx`, payout, balance | 5 | Khalti docs issues [#91](https://github.com/khalti/docs.khalti.com/issues/91) (filed by PayBridgeNP), [#87](https://github.com/khalti/docs.khalti.com/issues/87), [#13](https://github.com/khalti/docs.khalti.com/issues/13), [#48](https://github.com/khalti/docs.khalti.com/issues/48); [blog](https://kokil.com.np/blog/laravel-khalti-and-esewa-nepal-payment-integration) (eSewa refunds are a "Manual dashboard process only"). |
| ConnectIPS certificate handling (.pfx, signing) | 2 | [Medium article](https://medium.com/@dixxantashrestha/the-most-confusing-part-of-connectips-is-not-the-api-its-the-certificate-e5bec4311fb3) (title only; the page returned 403); this repository's own README has a conversion procedure. |
| Community packages go stale after gateway changes | 2 | [rn-all-nepal-payment](https://github.com/CodeDemonOrg/rn-all-nepal-payment) (23 stars, no push since 2021); blog claim that tutorials "still reference deprecated endpoints" ([blog](https://kokil.com.np/blog/laravel-khalti-and-esewa-nepal-payment-integration)). |

Not found: public Facebook-group summaries, dev.to posts, or usable Stack Overflow questions (two old eSewa questions only: [iOS SDK](https://stackoverflow.com/questions/59050735/how-to-integrate-esewa-payment-gateway-for-ios-apps), [callback URL](https://stackoverflow.com/questions/58268718/create-callback-url-in-post-method-api-for-online-payment)).

Reading of the table: for a server library the top two themes are the missing push channel and access to credentials and sandboxes. Signature and unit problems are already handled by this project.

## 5. Synthesis

### 5.1 Candidate directions

Order is by strength of evidence, then by cost. It is not a recommendation. "Merchant developer" means someone who builds one shop or app with its own gateway accounts. "Platform builder" means someone who builds a licensed PSP, an aggregator under a PSP, or a SaaS for many merchants.

**1. Status lookup as a first-class call**

- What: a `Lookup` (or `CheckStatus`) method that needs only stored identifiers, not callback parameters, plus a "final or not" answer for each status, including eSewa `PENDING`/`AMBIGUOUS` and ConnectIPS "incomplete".
- Evidence: sections 3.1 and 3.2 (every supported gateway documents a lookup, none a webhook); section 4 theme 2; pay-np already has it (1.1).
- Serves: both.
- Size: small. eSewa, Khalti and ConnectIPS already call these endpoints inside `VerifyPayment`.
- Risks: Fonepay web lookup may be impossible without the redirect `UID` (3.1, not confirmed from Fonepay). An interface change is a breaking change for implementers of `Provider`.

**2. Local mock gateways for development and tests**

- What: an `httptest`-style fake of each gateway (form page, redirect, status endpoint, signatures) so that an application can run the full flow, including "customer closed the tab", without credentials.
- Evidence: section 3.1 sandbox column (ConnectIPS and Fonepay have no open sandbox; Khalti needs a merchant signup); section 4 themes on access and environments; this repository's README says "The FonePay sandbox is not always available".
- Serves: both, and students, who are a visible group in section 4.
- Size: medium. Standard library only. No storage.
- Risks: the fake drifts from the real gateway; behaviour for undocumented cases must be guessed and labelled.

**3. Payment record, state machine and "mark paid once"**

- What: a `Payment`/attempt type with states and legal transitions, a small `Store` interface, an in-memory store and one `database/sql` reference store. It keeps the Khalti `pidx`, the expected amount and the order ID, so that the checks the README asks users to do by hand happen inside the library.
- Evidence: section 2 (state machine, idempotency, event log are the common core everywhere); section 1 (only the Laravel package and Odoo addon have it; remotemerge states that applications must do it themselves); section 3.2 (Khalti and ConnectIPS need stored data to verify).
- Serves: merchant developers most. Platform builders usually have their own schema.
- Size: medium to large.
- Risks: ends "stateless". A schema is an opinion that users will want to change. A SQL reference store can stay on the standard library, but tests for it need a driver.

**4. Reconciler**

- What: a loop that takes unfinished payments from the store, calls direction 1 with backoff, and applies the result through direction 3. Runs as a goroutine or from the user's cron.
- Evidence: section 2 (Kill Bill Janitor); section 3.1 (eSewa step 6); section 4 theme 2; three community READMEs describe this job and only the Odoo addon ships it.
- Serves: both.
- Size: small once 1 and 3 exist; it depends on both.
- Risks: polling limits are not documented by any gateway (not confirmed), so default intervals are guesses. Expiry rules differ (Khalti 60 minutes; eSewa "some minutes").

**5. NepalQR payload encoder and parser**

- What: build and parse the EMVCo merchant-presented string of the NRB annex (tags, static/dynamic flag, amount, CRC), with no network calls.
- Evidence: section 3.3 (public standard, 2.9 million QR merchants); section 4 (developers ask why they cannot "just place the business QR"); the interest in the RojanSapkota project (1.3).
- Serves: merchant developers with an acquirer-issued QR; platform builders who are or work under an acquirer.
- Size: small to medium for the codec. Large if it includes NepalPay QR network calls.
- Risks: a correct string is not a payable QR unless the merchant identifiers are registered with an acquirer. Confirmation of a QR payment still needs an acquirer channel (Fonepay or NepalPay WebSocket, 3.1), which is exactly the part without open access. Users may expect more than the codec can do.

**6. Refund interface**

- What: an optional `Refunder` interface, implemented for Khalti; documented as unsupported elsewhere.
- Evidence: section 3.1 (one gateway has a merchant refund API); section 4 theme on missing merchant APIs; the status constants already include `REFUNDED`.
- Serves: merchant developers on Khalti.
- Size: small.
- Risks: Khalti's page does not state the HTTP method or auth and contradicts itself on units; behaviour must be tested against the sandbox. An interface with one implementation may mislead. v2 removed refund types from `core` once already.

**7. More providers**

- What: Hamro Pay Checkout (self-service sandbox, has a webhook and a lookup), then IME Pay or NPS OnePG if documents can be obtained.
- Evidence: section 3.1; pay-np already covers Hamro Pay and IME Pay (1.1).
- Serves: merchant developers.
- Size: small per provider.
- Risks: IME Pay's developer site now redirects to Khalti's, so its future as a separate API is not confirmed. OnePG and bank card gateways have no first-party public documents. Adding Hamro Pay would also be the first reason to add webhook verification helpers (section 2).

**8. Self-hosted checkout server**

- What: one binary with a database, a hosted checkout page that lists methods, the reconciler, and signed outbound webhooks to the merchant's application. The open-source, self-hosted form of what PayBridgeNP sells.
- Evidence: section 1.3 (the only orchestration product is hosted and paid; a hobby "bridge" has 27 stars after three months); section 2 (hosted checkout and webhooks are standard blocks); this serves non-Go applications too, which widens the audience beyond section 1's Go niche.
- Serves: merchant developers and agencies. For platform builders it is a starting point only.
- Size: very large; it contains directions 1, 3 and 4.
- Risks: a different product from a library, with operations, upgrades and security duties. The licence position of a multi-tenant deployment is open (3.5). A funded hosted competitor exists.

### 5.2 Trade-offs between them

- Directions 1, 2, 5, 6 and 7 keep the library stateless and standard-library only. Directions 3, 4 and 8 do not.
- Directions 3 and 4 answer the best-supported pain point completely; direction 1 alone answers it only for users who already have their own tables and jobs.
- Direction 2 is the only one that addresses the access barrier, which no library can remove.
- Direction 5 has the widest possible audience (QR is the dominant instrument) and the weakest link between what the code does and what the user wants (a confirmed payment).
- Direction 8 is the literal reading of "what a developer uses to build a payment system". It is also the one where a mistake costs users money.
- pay-np is ahead on directions 1 and 7. No project is ahead on 2, 3 plus 4 in Go, or 8 as open source.

### 5.3 Things not to build

| Do not build | Reason |
|---|---|
| Collection into one account and payout to sub-merchants, split payments, escrow | This is "intermediate between customer and service provider", the PSP activity (3.4). It needs an NRB licence. |
| Wallet or stored-value balance, or a ledger of money owed to others | Same reason. A ledger also has no purpose when the system holds no funds (section 2). |
| Payment detection by reading email, SMS or bank notifications, or QR codes that point at a personal wallet ID | No gateway documents it; it depends on message formats that can change; matching by amount can credit the wrong order. Use of personal accounts for sales is outside the merchant enrolment that NRB's QR framework assumes (3.3). |
| NepalPay QR issuer or acquirer APIs (validate, payment, settlement reports), switch functions | These are for network members: banks and PSPs (3.1). "Payment Switch/Gateway" is a PSO activity (3.4). |
| Routing and automatic fallback between providers | The customer chooses the wallet or bank. The gateways are not interchangeable processors of one card, which is the case that Hyperswitch routing assumes (section 2). |
| Card vault or raw card handling | No Nepali card gateway has public documents (3.1), and card data brings PCI duties (section 2). |
| Subscriptions and recurring billing | No gateway in section 3.1 documents a stored-credential or mandate API. Not confirmed that one exists. |
| A multi-tenant hosted service run by this project | Licence position not confirmed (3.5); it is a business, not a library. |
| Native mobile SDK wrappers | The largest issue count in section 4, but the cause is the vendors' binary SDKs, and it is far from a Go server library. |

## 6. Questions I could not answer

1. What the Payment and Settlement Act 2075 says exactly about operating without a licence, and the penalty. The Act is published in Nepali only and the download failed.
2. The activity lists and capital amounts in the current Licensing Policy (2023, amended 2024), and whether it names payment gateways, aggregators or technology service providers.
3. Whether the Unified Directives 2024 set merchant settlement times, refund deadlines, or rules for third-party technology providers.
4. Whether a SaaS that stores merchants' gateway credentials and initiates payments, without touching funds, needs NRB approval.
5. Fonepay: official documents for the web status check, QR status WebSocket, refund API, settlement schedule and sandbox. Everything known is from community code and one vendor blog.
6. eSewa: whether a merchant refund API or a statement API exists outside the public ePay page, and what the other pages of the developer site contain (the navigation did not render).
7. Khalti: the HTTP method, auth and amount unit of the refund endpoint; whether any merchant can call it or only by agreement.
8. Rate limits for status lookups at any gateway.
9. IME Pay: whether its merchant API still exists after the move to Khalti's documentation.
10. Whether a merchant can call NepalPay QR `generateQR` and the WebSocket directly, or only through its acquirer.
11. First-party integration documents for Nabil, Himalayan Bank and other card gateways, and for NPS OnePG.
12. Whether QR codes of the Fonepay, NepalPay and Smart QR networks are payable across all apps today.
13. The content of the Reddit threads beyond titles and snippets, and any Facebook-group discussion.
14. Whether the "refund" and "subscription" methods of np-payment-sdk and the newer npm packages do real work for Nepali gateways. I read READMEs, not their code.
