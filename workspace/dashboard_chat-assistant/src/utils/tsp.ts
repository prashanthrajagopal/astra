export async function computeTSPSolution(cities: { x: number; y: number }[]) {
  // Placeholder for TSP solution computation logic
  // For demonstration, we'll return a dummy solution
  const sortedCities = cities.sort(() => Math.random() - 0.5);
  return {
    path: sortedCities.map((city, index) => city.x + "," + city.y),
    cost: 0.5 * sortedCities.length,
  };
}