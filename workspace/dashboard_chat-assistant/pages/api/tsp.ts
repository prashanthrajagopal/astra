import { NextResponse } from "next/server";
import { computeTSPSolution } from "@/utils/tsp";

export const POST = async (req: Request) => {
  try {
    const body = await req.json();
    if (!body || typeof body !== "object" || !Array.isArray(body.cities)) {
      return NextResponse.json({ error: "Invalid request payload" }, { status: 400 });
    }

    const cities = body.cities.map((city) => ({ x: city.x, y: city.y }));
    const solution = await computeTSPSolution(cities);

    return NextResponse.json(solution);
  } catch (error) {
    console.error("Error computing TSP solution:", error);
    return NextResponse.json({ error: "An error occurred while computing the TSP solution" }, { status: 500 });
  }
};