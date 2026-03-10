import validate from './validate';

export default async function handler(req: NextApiRequest, res: NextApiResponse) {
  try {
    await validate(req, res);
  } catch (error) {
    res.status(500).json({ error: 'Internal Server Error' });
  }
}