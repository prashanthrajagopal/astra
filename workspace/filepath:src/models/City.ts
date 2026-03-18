export class City {
    id: number;
    name: string;
    coordinates: { x: number; y: number };

    constructor(id: number, name: string, coordinates: { x: number; y: number }) {
        this.id = id;
        this.name = name;
        this.coordinates = coordinates;
    }
}