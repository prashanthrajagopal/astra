#!/usr/bin/env python3.10
from typing import Callable, IO

def add_three_numbers() -> IO[str]:
    def get_number(prompt: str) -> int:
        while True:
            try:
                return int(input(prompt))
            except ValueError:
                print("Invalid input. Please enter a number.")

    num1 = get_number("Enter the first number: ")
    num2 = get_number("Enter the second number: ")
    num3 = get_number("Enter the third number: ")

    return f"The sum of the three numbers is: {num1 + num2 + num3}"