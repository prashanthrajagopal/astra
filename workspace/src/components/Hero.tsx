import { Link } from 'next/link';
import { Container, Text, Button } from '@clayui/core';

const Hero = () => {
  return (
    <Container fluid={true} style={{ height: '100vh', backgroundColor: '#333' }}>
      <Container fluid={true} style={{ height: '100vh', backgroundColor: 'rgba(0,0,0,0.5)' }} className="d-flex flex-column justify-content-center align-items-center">
        <Text h1 className="text-light">Welcome to My App</Text>
        <Text className="text-light">
          Explore our featured products and discover new ones
        </Text>
        <Button
          theme="primary"
          size="lg"
          className="mt-4"
          as={Link}
          href="/products"
        >
          Shop Now
        </Button>
      </Container>
    </Container>
  );
};

export default Hero;