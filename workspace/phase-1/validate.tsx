import Head from 'next/head';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers';
import { schema } from '../lib/validate.schema';

const Validate = () => {
  const { register, handleSubmit, errors } = useForm({
    resolver: yupResolver(schema),
  });

  const [results, setResults] = useState([]);

  const onSubmit = async (data) => {
    try {
      const response = await fetch('/api/validate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      });
      const resultsData = await response.json();
      setResults(resultsData);
    } catch (error) {
      setResults([{ id: 'Error', message: error.message }]);
    }
  };

  return (
    <div>
      <Head>
        <title>Validate Phase 1</title>
      </Head>
      <h1>Validate Phase 1</h1>
      <form onSubmit={handleSubmit(onSubmit)}>
        <div>
          <label>
            Name:
            <input type="text" {...register('name')} />
            {errors.name && <span>{errors.name.message}</span>}
          </label>
          <label>
            Email:
            <input type="email" {...register('email')} />
            {errors.email && <span>{errors.email.message}</span>}
          </label>
        </div>
        <button type="submit">Validate</button>
      </form>
      {results.length > 0 && (
        <ul>
          {results.map((result) => (
            <li key={result.id}>
              {result.id}: {result.message}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};

export default Validate;