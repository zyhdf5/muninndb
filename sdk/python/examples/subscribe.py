"""SSE subscription demo — listen for new engrams written to vault.

This example demonstrates the real-time push capability:
1. Subscribe to a vault via Server-Sent Events (SSE)
2. Write a new engram in a concurrent task
3. Receive push notification when engram is written

Run with: python examples/subscribe.py
"""

import asyncio

from muninn import MuninnClient


async def main():
    """Subscribe and write example."""
    async with MuninnClient("http://localhost:8476") as client:
        print("MuninnDB SSE Subscription Demo\n")

        # Task to write an engram after 1 second
        async def writer():
            await asyncio.sleep(1)
            print("↳ Writing engram...")
            eid = await client.write(
                vault="default",
                concept="subscription test",
                content="This write should trigger the subscriber to receive a push event.",
                tags=["test", "demo"],
                confidence=0.9,
            )
            print(f"↳ Written: {eid}\n")

        # Start writer task concurrently
        writer_task = asyncio.create_task(writer())

        # Subscribe to vault
        print("Subscribing to 'default' vault...")
        stream = client.subscribe(vault="default", push_on_write=True)

        try:
            # Listen for pushes (stop after receiving 2)
            push_count = 0
            async for push in stream:
                push_count += 1
                print(f"Push #{push_count} received!")
                print(f"  subscription_id: {push.subscription_id}")
                print(f"  trigger: {push.trigger}")
                print(f"  push_number: {push.push_number}")
                print(f"  engram_id: {push.engram_id}")
                if push.at:
                    print(f"  at: {push.at}")

                # Stop after second push
                if push_count >= 2:
                    print("\nClosing subscription...")
                    await stream.close()
                    break

        finally:
            # Ensure writer task completes
            await writer_task


if __name__ == "__main__":
    asyncio.run(main())
