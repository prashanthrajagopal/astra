import React from 'react';
import { useForm } from 'react-hook-form';
import { useState } from 'react';
import { apiValidation } from '../api/api';

interface IFormData {
  name: string;
  email: string;
}

const ApiValidationForm = () => {
  const { register, handleSubmit } = useForm<IFormData>();
  const [validated, setValidated] = useState(false);

  const onSubmit = async (data: IFormData) => {
    await apiValidation(data);
  };

  const handleFormSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    event.stopPropagation();
    handleSubmit(onSubmit);
    setValidated(true);
  };

  return (
    <div>
      <form onSubmit={handleFormSubmit}>
        <div className="mb-3">
          <label className="form-label">Name</label>
          <input type="text" {...register('name')} />
        </div>
        <div className="mb-3">
          <label className="form-label">Email</label>
          <input type="email" {...register('email')} />
        </div>
        <button type="submit" className="btn btn-primary">
          Validate API
        </button>
      </form>
      {validated && (
        <div className="alert alert-success" role="alert">
          API request is valid!
        </div>
      )}
    </div>
  );
};

export default ApiValidationForm;