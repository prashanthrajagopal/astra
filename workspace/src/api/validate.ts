import axios from 'axios';
import { NextApiRequest, NextApiResponse } from 'next';

interface ValidateRequest {
  id: string;
}

interface ValidateResponse {
  isValid: boolean;
}

const validateApi = async (req: NextApiRequest, res: NextApiResponse) => {
  try {
    const { id } = req.body as ValidateRequest;
    const response = await axios.post('/api/validation', { id });
    const { isValid } = response.data as ValidateResponse;
    return res.status(200).json({ isValid });
  } catch (error) {
    return res.status(500).json({ message: 'Error validating API' });
  }
};

export default validateApi;