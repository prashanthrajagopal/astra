import React from 'react';

export default function GreatestOfThree() {
  const [num1, setNum1] = React.useState<number>(0);
  const [num2, setNum2] = React.useState<number>(0);
  const [num3, setNum3] = React.useState<number>(0);
  const [greatest, setGreatest] = React.useState<number | null>(null);

  const handleCheck = () => {
    setGreatest(Math.max(num1, num2, num3));
  };

  return (
    <div className="p-8">
      <h1 className="mb-4">Greatest of 3 Numbers</h1>
      <input
        type="number"
        value={num1}
        onChange={(e) => setNum1(Number(e.target.value))}
        className="w-full mb-4 p-2 border border-gray-300 rounded"
      />
      <input
        type="number"
        value={num2}
        onChange={(e) => setNum2(Number(e.target.value))}
        className="w-full mb-4 p-2 border border-gray-300 rounded"
      />
      <input
        type="number"
        value={num3}
        onChange={(e) => setNum3(Number(e.target.value))}
        className="w-full mb-4 p-2 border border-gray-300 rounded"
      />
      <button
        onClick={handleCheck}
        className="p-2 bg-blue-500 text-white rounded"
      >
        Check
      </button>
      {greatest !== null && (
        <p className="mt-4">The greatest number is: {greatest}</p>
      )}
    </div>
  );
}