import React from 'react';
import { useForm } from 'react-hook-form';

interface ValidationEditorProps {
  children: React.ReactNode;
}

const ValidationEditor: React.FC<ValidationEditorProps> = ({ children }) => {
  const { register, handleSubmit } = useForm();

  const onSubmit = async (data: any) => {
    const validationResult = await validateApi(data.input);
    if (validationResult.isValid) {
      alert('Validation successful!');
    } else {
      alert('Validation failed: ' + validationResult.message);
    }
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)}>
      {children}
      <button type="submit">Validate</button>
    </form>
  );
};

export default ValidationEditor;