import { GeoCoordinates } from './types';

const radiusOfEarthKm = 6371;

function toRadians(degrees: number) {
  return degrees * (Math.PI / 180);
}

function calculateDistance(lat1: number, lon1: number, lat2: number, lon2: number) {
  const dLat = toRadians(lat2 - lat1);
  const dLon = toRadians(lon2 - lon1);

  lat1 = toRadians(lat1);
  lat2 = toRadians(lat2);

  const a =
    Math.sin(dLat / 2) * Math.sin(dLat / 2) +
    Math.sin(dLon / 2) * Math.sin(dLon / 2) * Math.cos(lat1) * Math.cos(lat2);
  const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));

  return radiusOfEarthKm * c;
}

export function getDistanceBetweenCities(city1: GeoCoordinates, city2: GeoCoordinates): number {
  return calculateDistance(city1.lat, city1.lon, city2.lat, city2.lon);
}

export interface GeoCoordinates {
  lat: number;
  lon: number;
}