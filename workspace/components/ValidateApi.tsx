import React, { useState } from 'react';
import { useNavigate } from 'next/router';

interface ValidateApiProps {
  apiEndpoint: string;
  apiMethod: 'GET' | 'POST' | 'PUT' | 'DELETE';
  apiHeaders?: { [key: string]: string };
  apiQueryParams?: { [key: string]: string };
}

const ValidateApi: React.FC<ValidateApiProps> = ({
  apiEndpoint,
  apiMethod,
  apiHeaders = {},
  apiQueryParams = {},
}) => {
  const [response, setResponse] = useState(null);
  const navigate = useNavigate();

  const validateApi = async () => {
    try {
      const requestHeaders = { ...apiHeaders };
      const requestQueryParams = { ...apiQueryParams };

      switch (apiMethod) {
        case 'GET':
          const responseGet = await fetch(`${apiEndpoint}?${Object.keys(requestQueryParams)
            .map((key) => `${key}=${requestQueryParams[key]}`)
            .join('&')}`, {
            headers: requestHeaders,
          });
          const dataGet = await responseGet.json();
          setResponse(dataGet);
          break;
        case 'POST':
          const responsePost = await fetch(apiEndpoint, {
            method: 'POST',
            headers: requestHeaders,
            body: JSON.stringify(requestQueryParams),
          });
          const dataPost = await responsePost.json();
          setResponse(dataPost);
          break;
        case 'PUT':
          const responsePut = await fetch(`${apiEndpoint}?${Object.keys(requestQueryParams)
            .map((key) => `${key}=${requestQueryParams[key]}`)
            .join('&')}`, {
            method: 'PUT',
            headers: requestHeaders,
          });
          const dataPut = await responsePut.json();
          setResponse(dataPut);
          break;
        case 'DELETE':
          const responseDelete = await fetch(apiEndpoint, {
            method: 'DELETE',
            headers: requestHeaders,
          });
          const dataDelete = await responseDelete.json();
          setResponse(dataDelete);
          break;
        default:
          throw new Error('Invalid API method');
      }

      if (response) {
        navigate('/api/validated');
      }
    } catch (error) {
      console.error(error);
      setResponse(null);
    }
  };

  return (
    <div>
      <button onClick={validateApi}>Validate API</button>
      {response && (
        <div>
          <h2>API Response:</h2>
          <pre>{JSON.stringify(response, null, 2)}</pre>
        </div>
      )}
    </div>
  );
};

export default ValidateApi;