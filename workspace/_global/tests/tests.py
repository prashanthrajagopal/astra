tests/tests.py
import unittest
from app.app import app

class TestApp(unittest.TestCase):
    def setUp(self):
        self.app = app.test_client()

    def test_get_test(self):
        response = self.app.get('/test')
        self.assertEqual(response.status_code, 200)
        self.assertGreaterEqual(len(response.json), 1)

    def test_create_test(self):
        data = {'name': 'Test 1'}
        response = self.app.post('/test', json=data)
        self.assertEqual(response.status_code, 201)
        self.assertEqual(response.json['name'], data['name'])
