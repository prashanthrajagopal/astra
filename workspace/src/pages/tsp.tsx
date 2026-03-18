import React, { useEffect, useState } from 'react';
import axios from 'axios';

const TSPSolution = () => {
  const [solution, setSolution] = useState(null);

  useEffect(() => {
    fetchTSPSolution();
  }, []);

  const fetchTSPSolution = async () => {
    try {
      const response = await axios.get('http://localhost:3000/tsp');
      setSolution(response.data);
    } catch (error) {
      console.error('Error fetching TSP solution:', error);
    }
  };

  return (
    <div className="p-8 max-w-screen-md mx-auto">
      <h1 className="text-3xl font-bold mb-4">Traveling Salesperson Problem Solution</h1>
      {solution ? (
        <div className="bg-white p-4 rounded shadow">
          <pre className="language-javascript">{JSON.stringify(solution, null, 2)}</pre>
        </div>
      ) : (
        <p>Loading...</p>
      )}
    </div>
  );
};

export default TSPSolution;