import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const breakfastData = require("../data/breakfast.json");
const lunchData = require("../data/lunch.json");
const dinnerData = require("../data/dinner.json");

export const config = {
  matcher: "/api/meal/*path",
};

export function GET(request: NextRequest) {
  const path = request.nextUrl.pathname.split("/api/meal/")[1];
  switch (path) {
    case "breakfast":
      return NextResponse.json(breakfastData);
    case "lunch":
      return NextResponse.json(lunchData);
    case "dinner":
      return NextResponse.json(dinnerData);
    default:
      return NextResponse.json({ error: "Invalid meal type" }, { status: 400 });
  }
}