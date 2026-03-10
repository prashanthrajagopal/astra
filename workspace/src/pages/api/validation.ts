import axios from 'axios';

interface ValidateResponse {
  isValid: boolean;
}

const validationApi = async () => {
  try {
    const response = await axios.get('https://api.example.com/validate');
    return response.data as ValidateResponse;
  } catch (error) {
    return { isValid: false };
  }
};

export default validationApi;