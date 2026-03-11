import { useState } from 'react';

interface ShippingInformation {
  name: string;
  email: string;
  address: string;
  city: string;
  zip: string;
}

const ShippingInformationForm = ({
  shippingInformation,
  setShippingInformation,
}: {
  shippingInformation: ShippingInformation;
  setShippingInformation: React.Dispatch<React.SetStateAction<ShippingInformation>>;
}) => {
  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setShippingInformation({
      ...shippingInformation,
      [event.target.name]: event.target.value,
    });
  };

  return (
    <form className="flex flex-col gap-4">
      <label className="text-sm font-bold">
        Name:
        <input
          type="text"
          name="name"
          value={shippingInformation.name}
          onChange={handleInputChange}
          className="text-sm"
        />
      </label>
      <label className="text-sm font-bold">
        Email:
        <input
          type="email"
          name="email"
          value={shippingInformation.email}
          onChange={handleInputChange}
          className="text-sm"
        />
      </label>
      <label className="text-sm font-bold">
        Address:
        <input
          type="text"
          name="address"
          value={shippingInformation.address}
          onChange={handleInputChange}
          className="text-sm"
        />
      </label>
      <label className="text-sm font-bold">
        City:
        <input
          type="text"
          name="city"
          value={shippingInformation.city}
          onChange={handleInputChange}
          className="text-sm"
        />
      </label>
      <label className="text-sm font-bold">
        Zip:
        <input
          type="text"
          name="zip"
          value={shippingInformation.zip}
          onChange={handleInputChange}
          className="text-sm"
        />
      </label>
    </form>
  );
};

export default ShippingInformationForm;