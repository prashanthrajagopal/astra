import React from 'react';

interface Props {
  handleSubmit: (data: any) => void;
}

const ValidateForm: React.FC<Props> = ({ handleSubmit }) => {
  const [data, setData] = React.useState({
    username: '',
    email: '',
  });

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setData({
      ...data,
      [event.target.name]: event.target.value,
    });
  };

  const handleFormSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    handleSubmit(data);
  };

  return (
    <form onSubmit={handleFormSubmit}>
      <label>
        Username:
        <input type="text" name="username" value={data.username} onChange={handleInputChange} />
      </label>
      <br />
      <label>
        Email:
        <input type="email" name="email" value={data.email} onChange={handleInputChange} />
      </label>
      <br />
      <button type="submit">Validate</button>
    </form>
  );
};

export default ValidateForm;