import React from 'react';

interface ValidationCardProps {
  isValid: boolean;
}

const ValidationCard: React.FC<ValidationCardProps> = ({ isValid }) => {
  return (
    <div className="flex justify-center py-10">
      {isValid ? (
        <div className="bg-green-500 rounded py-2 px-4 text-white text-lg">
          Valid
        </div>
      ) : (
        <div className="bg-red-500 rounded py-2 px-4 text-white text-lg">
          Invalid
        </div>
      )}
    </div>
  );
};

export default ValidationCard;