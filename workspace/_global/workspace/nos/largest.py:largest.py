#!/usr/bin/env python3
from typing import Optional

def largest(a: int, b: int) -> Optional[int]:
    """Return the largest of two integers."""
    return max(a, b)

if __name__ == "__main__":
    import sys
    if len(sys.argv) != 3:
        print("Usage: python largest.py <num1> <num2>")
        sys.exit(1)
    num1 = int(sys.argv[1])
    num2 = int(sys.argv[2])
    print(f"The largest number is {largest(num1, num2)}")