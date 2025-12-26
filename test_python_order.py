#!/usr/bin/env python3
"""
Test order placement using Polymarket Python client.
Based on official py-clob-client examples.
"""
import os
from py_clob_client.client import ClobClient
from py_clob_client.clob_types import ApiCreds, OrderArgs, OrderType
from py_clob_client.order_builder.constants import BUY

# Load environment variables
def load_env():
    """Load .env file from current directory."""
    env_vars = {}
    with open('.env', 'r') as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#') and '=' in line:
                key, value = line.split('=', 1)
                env_vars[key] = value
    return env_vars

def main():
    env = load_env()

    # Configuration
    HOST = "https://clob.polymarket.com"
    CHAIN_ID = 137  # Polygon mainnet
    PRIVATE_KEY = env.get("POLYMARKET_PRIVATE_KEY")

    # API credentials (for Level 2 auth)
    api_key = env.get("POLYMARKET_API_KEY")
    api_secret = env.get("POLYMARKET_SECRET")
    api_passphrase = env.get("POLYMARKET_PASSPHRASE")

    print("=== Testing Python Polymarket Client ===\n")
    print(f"API Key: {api_key}")
    print(f"Private Key: {PRIVATE_KEY[:10]}...")

    # Create API credentials object
    creds = ApiCreds(
        api_key=api_key,
        api_secret=api_secret,
        api_passphrase=api_passphrase
    )

    # Initialize client (EOA mode - signature_type=0 is default)
    # For EOA wallets, do NOT set signature_type=1 or funder parameter
    client = ClobClient(
        HOST,
        key=PRIVATE_KEY,
        chain_id=CHAIN_ID,
        creds=creds
        # signature_type defaults to 0 (EOA)
        # funder is not set (same as signer for EOA)
    )

    # Token IDs from previous Go test
    yes_token_id = "11862165566757345985240476164489718219056735011698825377388402888080786399275"

    print(f"\nYES Token: {yes_token_id[:20]}...")

    # Order parameters (matching Go test)
    price = 0.15
    size = 1.1

    print(f"\n=== Building Order ===")
    print(f"Price: ${price}")
    print(f"Size: ${size}")
    print(f"Side: BUY")

    try:
        # Create order arguments
        order_args = OrderArgs(
            token_id=yes_token_id,
            price=price,
            size=size,
            side=BUY,
        )

        # Build and sign order
        print("\nBuilding and signing order...")
        signed_order = client.create_order(order_args)

        print(f"✓ Order signed successfully")

        # Print order details for comparison
        order_dict = signed_order.dict()
        print("\n=== PYTHON ORDER STRUCTURE ===")
        print(f"Salt: {order_dict['salt']}")
        print(f"Maker: {order_dict['maker']}")
        print(f"Signer: {order_dict['signer']}")
        print(f"Taker: {order_dict['taker']}")
        print(f"TokenId: {order_dict['tokenId']}")
        print(f"MakerAmount: {order_dict['makerAmount']}")
        print(f"TakerAmount: {order_dict['takerAmount']}")
        print(f"Side: {order_dict['side']}")
        print(f"FeeRateBps: {order_dict['feeRateBps']}")
        print(f"Nonce: {order_dict['nonce']}")
        print(f"Expiration: {order_dict['expiration']}")
        print(f"SignatureType: {order_dict['signatureType']}")
        print(f"Signature: {signed_order.signature}")
        print("=" * 50)

        # Place order
        print("\nPlacing order...")
        response = client.post_order(signed_order, OrderType.GTC)

        print("\n=== Order Response ===")
        print(f"Success: {response.get('success', False)}")
        print(f"Order ID: {response.get('orderID', '')}")
        print(f"Error: {response.get('errorMsg', '')}")
        print(f"Status: {response.get('status', '')}")

        if response.get('errorMsg'):
            print(f"\n❌ ORDER FAILED: {response['errorMsg']}")
            return 1
        elif response.get('orderID'):
            print(f"\n✅ ORDER PLACED SUCCESSFULLY")
            print(f"Order ID: {response['orderID']}")
            return 0
        else:
            print(f"\n⚠️  UNKNOWN RESULT")
            print(f"Full response: {response}")
            return 2

    except Exception as e:
        print(f"\n❌ EXCEPTION: {e}")
        import traceback
        traceback.print_exc()
        return 3

if __name__ == "__main__":
    exit(main())
