# Privacy Policy

_Last updated: 29 October 2026_

matchblox is a console for running and steering your coding agents. **Matchblox
Pocket** is its app for iPhone and iPad. This policy explains what the app does
and does not collect. The short version: there is no account, we do not track
you, and we do not sell data. Your sessions stay between your device and the
hosts you pair with.

## No account, no profile

Matchblox Pocket has no sign-up, no login, and no password. We do not create a
user profile, and we do not ask for your name, email, or contacts. Pairing with
a host — by scanning a QR code — is the only setup, and it is the consent.

## What stays on your device

- **Camera.** Used only to scan a pairing QR code. Frames are processed on your
  device and are never stored or transmitted.
- **Face ID / Touch ID.** Handled entirely by iOS to unlock the key that opens
  your console. Biometric data never leaves your device and is never seen by us.
- **Keys.** Your SSH key and the app's device keys live in the device keychain
  (Secure Enclave–backed where available). They are not uploaded to us.

## Your sessions

When you open a host, the app connects to it **directly over SSH**, from your
device to the host you paired with. The contents of your terminal sessions are
not sent to matchblox servers, to any third party, or to any AI model.

## Notifications (the "buzz")

So a waiting agent session can reach you, the app registers a **push token** and
a **device attestation** (Apple's App Attest) with the matchblox push relay.
To deliver a notification, your host **seals its contents to a key that only
your device holds** (end-to-end encryption). As a result:

- Apple's Push Notification service and the matchblox relay carry only
  **ciphertext and a content-free placeholder** — not the session name, the
  question, or any message text. Your device opens the sealed line locally.
- The relay handles only **delivery metadata** needed to send a push (a push
  token or Live Activity channel id, and the requesting IP address, used to
  rate-limit abuse). It does not store your message content.

## What we do not do

- We do not use third-party analytics, advertising, or tracking SDKs.
- We do not track you across apps or websites.
- We do not sell or rent your data.

## Data shared with Apple

Push delivery uses Apple's Push Notification service (encrypted payloads, as
above). Device attestation uses Apple's App Attest. These are governed by
Apple's own privacy terms.

## Children

Matchblox Pocket is a developer tool and is not directed to children under 13.

## Changes

We may update this policy as the app evolves. Material changes will be noted
here with a new "last updated" date.

## Contact

Questions about privacy: **martin@yankovs.com**. Security reports: use GitHub
private vulnerability reporting at
https://github.com/blox-eng/matchblox/security/advisories/new (see SECURITY.md).
