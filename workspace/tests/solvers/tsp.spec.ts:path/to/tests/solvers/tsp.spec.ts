import { TSPSolver } from '@/solvers/tsp';
import { Point, solveTSP } from 'tsp-solver'; // Assuming tsp-solver is a package for TSP

describe('TSP Solver', () => {
  let solver: TSPSolver;

  beforeEach(() => {
    solver = new TSPSolver();
  });

  test('should solve the TSP with a simple set of points', () => {
    const points = [
      new Point({ x: 0, y: 0 }),
      new Point({ x: 1, y: 0 }),
      new Point({ x: 1, y: 1 })
    ];
    const result = solver.solve(points);
    expect(result.path.length).toBe(3);
    expect(result.path[0].x).toBe(0);
    expect(result.path[0].y).toBe(0);
  });

  test('should handle a larger set of points', () => {
    const points = [
      new Point({ x: 0, y: 0 }),
      new Point({ x: 10, y: 20 }),
      new Point({ x: 30, y: 40 }),
      new Point({ x: 50, y: 60 })
    ];
    const result = solver.solve(points);
    expect(result.path.length).toBe(4);
  });

  test('should handle a point set with duplicate points', () => {
    const points = [
      new Point({ x: 0, y: 0 }),
      new Point({ x: 0, y: 1 })
    ];
    const result = solver.solve(points);
    expect(result.path.length).toBe(2);
  });

  test('should handle an empty point set', () => {
    const points = [];
    const result = solver.solve(points);
    expect(result.path.length).toBe(0);
  });
});