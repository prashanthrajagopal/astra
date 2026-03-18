type City = {
  id: number;
  x: number;
  y: number;
};

function calculateDistance(city1: City, city2: City): number {
  return Math.sqrt(Math.pow(city1.x - city2.x, 2) + Math.pow(city1.y - city2.y, 2));
}

function nearestNeighborAlgorithm(cities: City[]): number[] {
  const cityCount = cities.length;
  if (cityCount <= 1) return cities.map(city => city.id);

  const startCity = cities[0];
  let tour: number[] = [startCity.id];

  for (let i = 0; i < cityCount - 1; i++) {
    let nearestCity = null;
    let minDistance = Infinity;

    for (const city of cities.filter(city => !tour.includes(city.id))) {
      const distance = calculateDistance({ x: startCity.x, y: startCity.y }, city);
      if (distance < minDistance) {
        nearestCity = city;
        minDistance = distance;
      }
    }

    tour.push(nearestCity.id);
    startCity = nearestCity;
  }

  return tour;
}

export { City, calculateDistance, nearestNeighborAlgorithm };