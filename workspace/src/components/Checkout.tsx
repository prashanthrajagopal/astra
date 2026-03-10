import { useState } from 'react';
import { useTheme } from 'next-themes';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'next-intl';
import { Divider, Fieldset, Grid, Heading, Input, Label, Select, Textarea } from '@chakra-ui/react';
import { ChevronRightIcon } from '@chakra-ui/icons';

interface ShippingFormData {
  name: string;
  address: string;
  city: string;
  state: string;
  zip: string;
  country: string;
  phone: string;
}

interface OrderFormData {
  items: string[];
  subtotal: number;
  tax: number;
  total: number;
}

const Checkout: React.FC<{
  orderSummary: OrderFormData;
  setOrderSummary: (summary: OrderFormData) => void;
  handlePlaceOrder: () => void;
}> = ({ orderSummary, setOrderSummary, handlePlaceOrder }) => {
  const { theme } = useTheme();
  const { t } = useTranslation();
  const { handleSubmit, reset } = useForm<ShippingFormData>();

  const [shippingForm, setShippingForm] = useState<ShippingFormData>({
    name: '',
    address: '',
    city: '',
    state: '',
    zip: '',
    country: '',
    phone: '',
  });

  const handleShippingFormChange = (data: ShippingFormData) => {
    setShippingForm(data);
  };

  const handleOrderSummaryUpdate = () => {
    setOrderSummary({
      items: ['Item 1', 'Item 2'],
      subtotal: 50,
      tax: 5,
      total: 55,
    });
  };

  const handlePlaceOrderSubmit = (data: ShippingFormData) => {
    const orderSummary: OrderFormData = {
      items: data.items,
      subtotal: 50,
      tax: 5,
      total: 55,
    };
    setOrderSummary(orderSummary);
    handlePlaceOrder();
  };

  return (
    <Grid templateColumns="repeat(2, 1fr)" gap={4}>
      <section>
        <Heading size="lg" mb={4}>
          {t('checkout.shipping')}
        </Heading>
        <form onSubmit={handleSubmit(handlePlaceOrderSubmit)}>
          <Fieldset
            title={t('checkout.shipping-info')}
            description={t('checkout.shipping-info-desc')}
          >
            <Label htmlFor="name">{t('checkout.name')}</Label>
            <Input
              id="name"
              type="text"
              value={shippingForm.name}
              onChange={(e) =>
                handleShippingFormChange({
                  ...shippingForm,
                  name: e.target.value,
                })
              }
            />
            <Label htmlFor="address">{t('checkout.address')}</Label>
            <Input
              id="address"
              type="text"
              value={shippingForm.address}
              onChange={(e) =>
                handleShippingFormChange({
                  ...shippingForm,
                  address: e.target.value,
                })
              }
            />
            <Label htmlFor="city">{t('checkout.city')}</Label>
            <Input
              id="city"
              type="text"
              value={shippingForm.city}
              onChange={(e) =>
                handleShippingFormChange({
                  ...shippingForm,
                  city: e.target.value,
                })
              }
            />
            <Label htmlFor="state">{t('checkout.state')}</Label>
            <Input
              id="state"
              type="text"
              value={shippingForm.state}
              onChange={(e) =>
                handleShippingFormChange({
                  ...shippingForm,
                  state: e.target.value,
                })
              }
            />
            <Label htmlFor="zip">{t('checkout.zip')}</Label>
            <Input
              id="zip"
              type="text"
              value={shippingForm.zip}
              onChange={(e) =>
                handleShippingFormChange({
                  ...shippingForm,
                  zip: e.target.value,
                })
              }
            />
            <Label htmlFor="country">{t('checkout.country')}</Label>
            <Input
              id="country"
              type="text"
              value={shippingForm.country}
              onChange={(e) =>
                handleShippingFormChange({
                  ...shippingForm,
                  country: e.target.value,
                })
              }
            />
            <Label htmlFor="phone">{t('checkout.phone')}</Label>
            <Input
              id="phone"
              type="text"
              value={shippingForm.phone}
              onChange={(e) =>
                handleShippingFormChange({
                  ...shippingForm,
                  phone: e.target.value,
                })
              }
            />
          </form>
        </section>
        <section>
          <Heading size="lg" mb={4}>
            {t('checkout.order-summary')}
          </Heading>
          <ul>
            {orderSummary.items.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </section>
      </Grid>
      <Divider />
      <Button
        onClick={handleOrderSummaryUpdate}
        variant="solid"
        colorScheme={theme === 'light' ? 'blue' : 'gray'}
      >
        <ChevronRightIcon /> {t('checkout.place-order')}
      </Button>
    </section>
  );
};

export default Checkout;