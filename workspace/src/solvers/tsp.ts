import { TSPCity } from './types';

export class TSPSolver {
  private cities: TSPCity[];

  constructor(cities: TSPCity[]) {
    this.cities = cities;
  }

  solve(): number {
    const startCity = this.cities[0];
    let currentCityIndex = 0;
    let totalDistance = 0;

    while (this.cities.length > currentCityIndex) {
      const currentCity = this.cities[currentCityIndex];
      let nextCity: TSPCity | null = null;
      let minDistance = Infinity;

      for (let i = 0; i < this.cities.length; i++) {
        if (this.cities[i] !== currentCity) {
          const distance = this.calculateDistance(currentCity, this.cities[i]);
          if (distance < minDistance) {
            nextCity = this.cities[i];
            minDistance = distance;
          }
        }
      }

      totalDistance += minDistance;
      currentCityIndex = this.cities.indexOf(nextCity!);
    }

    return totalDistance + this.calculateDistance(this.cities[currentCityIndex], startCity);
  }

  private calculateDistance(city1: TSPCity, city2: TSPCity): number {
    const xDiff = Math.abs(city1.x - city2.x);
    const yDiff = Math.abs(city1.y - city2.y);
    return xDiff + yDiff;
  }
}

interface TSPCity {
  id: number;
  x: number;
  y: number;
}