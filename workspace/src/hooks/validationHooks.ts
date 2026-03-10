import { useMutation } from 'react-query';
import { validateApi } from '../api/api';

interface ValidationHookProps {
  input: string;
}

const useValidation = (): {
  validate: (input: string) => Promise<ValidationResult>;
} => {
  const { mutate } = useMutation({
    mutationFn: validateApi,
  });

  return {
    validate: async (input: string) => mutate(input),
  };
};

export default useValidation;