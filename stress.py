import time
import requests
from concurrent.futures import ThreadPoolExecutor, as_completed

URL = "http://localhost:8080/api/sync/76561199801916052"
TOTAL = 50

API_KEY = "ZM1ng__80020113@"
USER_AGENT = "VHMing-API-Sync-Go/1.0"

HEADERS = {
    "X-API-KEY": API_KEY,
    "User-Agent": USER_AGENT,
}

RATE_HEADERS = [
    "RateLimit-Limit",
    "RateLimit-Remaining",
    "RateLimit-Reset",
    "X-RateLimit-Limit",
    "X-RateLimit-Remaining",
    "Retry-After",
]

def send_request(i):
    start = time.perf_counter()

    try:
        response = requests.get(
            URL,
            headers=HEADERS,
        )

        elapsed = (time.perf_counter() - start) * 1000

        result = [
            f"[{i:03d}/{TOTAL}] HTTP {response.status_code} | "
            f"{elapsed:.1f} ms"
        ]

        for key in RATE_HEADERS:
            value = response.headers.get(key)
            if value is not None:
                result.append(f"    {key}: {value}")

        if response.status_code == 429:
            result.append("    Rate limit detected!")

        return "\n".join(result)

    except requests.RequestException as exc:
        return f"[{i:03d}/{TOTAL}] Error: {exc}"


with ThreadPoolExecutor(max_workers=TOTAL) as executor:
    futures = [
        executor.submit(send_request, i)
        for i in range(1, TOTAL + 1)
    ]

    for future in as_completed(futures):
        print(future.result())