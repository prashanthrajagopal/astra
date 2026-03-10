import { NextApiRequest, NextApiResponse } from 'next';

const validate = async (req: NextApiRequest, res: NextApiResponse) => {
  const { name, email } = req.body;
  // validation logic here
  const results = [
    { id: 'Success', message: `Hello, ${name}! Your email is ${email}.` },
  ];
  return res.status(200).json(results);
};

export default validate;