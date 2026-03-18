import { TSPSolver } from "../../../src/solvers/tsp";

describe("TSP Solver", () => {
  const tspSolver = new TSPSolver();

  it("should solve the TSP for a simple case", () => {
    const distances = [
      [0, 10, 15],
      [10, 0, 35],
      [15, 35, 0]
    ];
    const expectedPath = [0, 1, 2];
    expect(tspSolver.solve(distances)).toEqual(expectedPath);
  });

  it("should handle an empty distance matrix", () => {
    const distances = [];
    expect(tspSolver.solve(distances)).toEqual([]);
  });

  it("should handle a single city", () => {
    const distances = [
      [0]
    ];
    expect(tspSolver.solve(distances)).toEqual([0]);
  });

  it("should handle a distance matrix with one city not connected to others", () => {
    const distances = [
      [0, 100],
      [100, 0]
    ];
    expect(tspSolver.solve(distances)).toEqual([0]);
  });

  it("should handle a distance matrix with multiple disconnected components", () => {
    const distances = [
      [0, 10],
      [10, 0],
      [50, 60],
      [60, 50]
    ];
    expect(tspSolver.solve(distances)).toEqual([0, 1]);
  });
});