# Releasing

Releases are built on GitHub and signed by iDev srl before they are published.

1. Update `CHANGELOG.md` and commit.
2. Tag and push: `git tag -a vX.Y.Z -m "CC Sessions X.Y.Z" && git push origin vX.Y.Z`.
3. The **Release** workflow builds `ccsessions.exe` from the tag on a clean runner and creates a **draft**
   release with the executable, a zip and `SHA256SUMS.txt`.
4. A maintainer at iDev srl downloads the executable from the draft and signs it with Authenticode
   (Azure Artifact Signing, iDev srl certificate, SHA-256, RFC 3161 timestamp). Only the release executable is
   signed; the zip contains that same signed file.
5. Verify the signature (`signtool verify /pa`, publisher `CN=iDev srl`, timestamp present), rebuild the zip and
   `SHA256SUMS.txt` around the signed executable, upload them to the draft and publish it.
6. Record the signature in [SIGNING-LOG.md](SIGNING-LOG.md).

Signing credentials never leave the maintainer's machine and are never stored in this repository or in CI.
