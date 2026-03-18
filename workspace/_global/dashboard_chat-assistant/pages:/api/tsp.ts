import { NextResponse } from 'next/server';
import { TSPSolution } from './tsp-solver';

export async function POST(request: Request) {
  const body = await request.json();
  const coordinates = body.coordinates;

  if (!coordinates || !Array.isArray(coordinates)) {
    return NextResponse.json({ error: 'Invalid coordinates' }, { status: 400 });
  }

  try {
    const solution = await TSPSolution(coordinates);
    return NextResponse.json(solution, { status: 200 });
  } catch (error) {
    return NextResponse.json({ error: 'Failed to solve TSP' }, { status: 500 });
  }
}