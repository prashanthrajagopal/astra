import { NextApiRequest, NextApiResponse } from 'next';
import { validate } from 'joi';

const schema = {
  body: {
    type: 'object',
    properties: {
      username: { type: 'string', minLength: 3, maxLength: 20 },
      email: { type: 'string', format: 'email' },
    },
    required: ['username', 'email'],
  },
};

export default async function validate(req: NextApiRequest, res: NextApiResponse) {
  try {
    const result = await validate(req.body, schema);
    if (!result.error) {
      res.status(201).json({ message: 'Validated successfully' });
    } else {
      res.status(400).json({ error: result.error.details });
    }
  } catch (error) {
    res.status(500).json({ error: 'Internal Server Error' });
  }
}