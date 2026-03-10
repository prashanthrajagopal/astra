import { useAppRouter } from 'next/app';
import { useState, useEffect } from 'react';

const validatePhase1 = () => {
  const router = useAppRouter();

  const [validationStatus, setValidationStatus] = useState({
    isValid: true,
    message: '',
  });

  useEffect(() => {
    // API call to validate phase 1
    const validate = async () => {
      try {
        const response = await fetch('/api/validate-phase-1');
        const data = await response.json();
        if (data.isValid) {
          setValidationStatus({ isValid: true, message: '' });
        } else {
          setValidationStatus({ isValid: false, message: data.message });
        }
      } catch (error) {
        setValidationStatus({ isValid: false, message: error.message });
      }
    };

    validate();
  }, []);

  return validationStatus;
};

export default validatePhase1;