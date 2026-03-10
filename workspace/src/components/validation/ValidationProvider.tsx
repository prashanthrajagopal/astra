import React from 'react';
import { useValidation } from '../hooks/validationHooks';

interface ValidationProviderProps {
  children: React.ReactNode;
}

const ValidationProvider: React.FC<ValidationProviderProps> = ({ children }) => {
  const { validate } = useValidation();

  return (
    <div>
      {children}
      <button onClick={() => validate('input value here')}>Validate</button>
    </div>
  );
};

export default ValidationProvider;