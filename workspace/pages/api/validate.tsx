import { NextApiRequest, NextApiResponse } from 'next';
import { validateRequest } from '../utils/validation';

const validate = async (req: NextApiRequest, res: NextApiResponse) => {
  try {
    const isValid = await validateRequest(req);
    if (isValid) {
      res.status(200).json({ message: 'Validation successful' });
    } else {
      res.status(400).json({ message: 'Validation failed' });
    }
  } catch (error) {
    res.status(500).json({ message: 'Error validating request' });
  }
};

export default validate;