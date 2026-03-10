import { NextApiRequest, NextApiResponse } from 'next';
import { validate } from 'joi';

const apiValidation = async (req: NextApiRequest, res: NextApiResponse) => {
  const apiSchema = {
    body: {
      type: 'object',
      required: ['name', 'email'],
      properties: {
        name: { type: 'string' },
        email: { type: 'string', format: 'email' }
      }
    }
  };

  try {
    await validate(req.body, apiSchema);
    res.status(200).json({ message: 'API request is valid' });
  } catch (error) {
    res.status(400).json({ message: 'API request is invalid', error: error.details });
  }
};

export default apiValidation;