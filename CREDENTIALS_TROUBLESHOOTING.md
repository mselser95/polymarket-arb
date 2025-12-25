# Polymarket API Credentials Troubleshooting

## Current Status: L1 Auth Working, L2 Auth Failing

✅ **L1 Authentication (derive-api-creds) is working!**
- Successfully deriving API credentials using EIP-712 signatures
- Credentials are cryptographically linked to private key

❌ **L2 Authentication (order submission) still getting 401**
- HMAC-SHA256 implementation is correct (verified against Python reference)
- Headers are correct (all required fields present)
- Signatures match expected format

## What We've Verified

✅ **Signature Generation is Correct**
- Using HMAC-SHA256 with base64url-decoded secret
- Message format: `timestamp + method + path + body`
- Signature is base64-encoded
- Matches Python reference implementation

✅ **Headers are Correct**
- POLY-API-KEY: API key from Builder Key
- POLY-SIGNATURE: HMAC signature
- POLY-TIMESTAMP: Unix seconds
- POLY-PASSPHRASE: Passphrase from Builder Key
- POLY-ADDRESS: Wallet address (0x4F7170A83fE02dF9AF3f7C5A8C8117692e3cb10C)

✅ **Credentials Format**
- API Key: UUID format
- Secret: Base64url encoded (28 chars + padding)
- Passphrase: 64-char hex string
- Private Key: 64-char hex (matches address)

## Possible Issues

### 1. Builder Key Permissions
**Symptom**: 401 "Unauthorized/Invalid api key"

**Possible Causes**:
- Builder Key might not have trading permissions
- Key might be for read-only access
- Key needs additional setup/activation

**Solution**: Check Polymarket dashboard → Builder Keys → Permissions

### 2. Wrong Wallet Address
**Symptom**: Credentials don't match wallet

**Check**:
- Builder Key created with wallet: `0x4F7170A83fE02dF9AF3f7C5A8C8117692e3cb10C`
- This matches your MetaMask private key

**If Mismatch**: Delete key, recreate while logged in with correct wallet

### 3. API Endpoint Issues
**Symptom**: All requests return 401

**Possible**:
- Endpoint requires additional registration
- Rate limiting/IP restrictions
- API in maintenance mode

### 4. Missing L1 Initialization
**Symptom**: L2 credentials don't work without L1 setup

**Possible**: Need to call L1 endpoint first to "activate" L2 credentials

**Try**: Use private key to call `/auth/derive-api-key` first

## Additional Possible Causes

### 5. Account Not Activated
**Symptom**: Derived credentials work for L1 but not L2
**Possible**: Account needs to make at least one trade via UI to activate
**Try**: Make a small trade ($1) via Polymarket.com web interface first

### 6. CLOB Allowances Not Set
**Symptom**: Authentication passes but orders fail
**Required**: Must approve CLOB contracts to spend tokens
**Setup**:
```bash
# Set allowances for USDC and CTF tokens
# - CTF Exchange: 0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E
# - Neg Risk Exchange: 0xC5d563A36AE78145C45a50134d48A1215220f80a
# - Neg Risk Adapter: 0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296
```
**Note**: Requires POL (Polygon MATIC) for gas fees on mainnet

### 7. Proxy Wallet vs EOA Mismatch
**Symptom**: Using proxy wallet (signature type 2) but credentials derived from EOA
**Details**:
- Your EOA: `0x4F7170A83fE02dF9AF3f7C5A8C8117692e3cb10C` (from private key)
- Your Proxy: `0x640CF75a03F91974eD4494B2c8FC42c727B1B6ef` (shown in UI)
- Signature Type: 2 (Gnosis Safe / multisig proxy)
**Try**: The derived credentials should work with EOA address in POLY-ADDRESS header

## Recommended Next Steps

### Option 1: Set CLOB Allowances
1. Ensure you have POL (Polygon MATIC) for gas
2. Set approvals for CTF and Neg Risk contracts
3. Test orders again

### Option 2: Make UI Trade First
1. Go to Polymarket.com
2. Make a small trade ($1-2) to activate account
3. Test API orders again

### Option 3: Contact Polymarket Support
- Show them the 401 error with your API key
- Ask if Builder Keys need special activation
- Verify endpoint is correct: `https://clob.polymarket.com/order`

### Option 2: Try Official SDK
Install their official client to verify credentials work:

```bash
npm install @polymarket/clob-client
# or
pip install py-clob-client
```

If official SDK also gets 401, it's definitely a credential/permissions issue.

### Option 3: Regenerate Credentials
1. Delete current Builder Key
2. Create NEW Builder Key
3. **Save all 3 values immediately** (only shown once!)
4. Update `.env` file
5. Test again

## Working Implementation

Our code is correctly implemented according to:
- [Polymarket Authentication Docs](https://docs.polymarket.com/developers/CLOB/authentication)
- [TypeScript Reference](https://github.com/Polymarket/clob-client/blob/main/src/signing/hmac.ts)
- [Python Reference](https://github.com/Polymarket/py-clob-client/blob/main/py_clob_client/signing/hmac.py)

Once credentials are valid, orders will:
1. ✅ Submit to API
2. ✅ Save successful responses to `test_responses/last_success_*.json`
3. ✅ Save error responses to `test_responses/last_error_*.json`
4. ✅ Display full debug output

## Working Commands

### Derive API Credentials (✅ WORKING)
```bash
# Derive credentials using L1 authentication (EIP-712)
go run . derive-api-creds

# Saves credentials to terminal - update your .env file with output:
# POLYMARKET_API_KEY=...
# POLYMARKET_SECRET=...
# POLYMARKET_PASSPHRASE=...
```

### Test Order Submission

```bash
# Paper mode (safe, no API) - ✅ WORKING
go run . test-live-order <market-slug> --paper

# Mock mode (saved responses) - ✅ WORKING
go run . test-live-order <market-slug> --mock

# Live mode (❌ Currently 401)
go run . test-live-order <market-slug> --live --size 1.0 --yes-price 0.01 --no-price 0.01
```

## Debug Output

Current debug output shows:
```
API Key: 019b5142-dcaf-7f1e-a217-1d2b5888473a
Address: 0x4F7170A83fE02dF9AF3f7C5A8C8117692e3cb10C
Timestamp: 1766614015
Signature: TnUmyCoBCfYSjaTW/g+2SHuLWk/k3Tj5rkOQfJA9Ssg=
```

Everything looks correct - this is a credential validation issue on Polymarket's side.
