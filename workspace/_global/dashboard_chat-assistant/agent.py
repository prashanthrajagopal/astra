import requests
import json

class AstraAgent:
    def __init__(self, api_key, api_url):
        self.api_key = api_key
        self.api_url = api_url

    def send_request(self, method, endpoint, data=None):
        headers = {'Authorization': f'Bearer {self.api_key}'}
        if method == 'post':
            response = requests.post(self.api_url + endpoint, json=data, headers=headers)
        elif method == 'get':
            response = requests.get(self.api_url + endpoint, headers=headers)
        elif method == 'put':
            response = requests.put(self.api_url + endpoint, json=data, headers=headers)
        elif method == 'delete':
            response = requests.delete(self.api_url + endpoint, headers=headers)
        else:
            raise ValueError('Invalid method')
        if response.status_code != 200:
            raise requests.exceptions.RequestException(f'Failed to {method} {endpoint}. Status code: {response.status_code}')
        return response.json()

    def get_tasks(self):
        return self.send_request('get', 'tasks')

    def create_task(self, task_data):
        return self.send_request('post', 'tasks', data=task_data)

    def get_task(self, task_id):
        return self.send_request('get', f'tasks/{task_id}')

    def update_task(self, task_id, task_data):
        return self.send_request('put', f'tasks/{task_id}', data=task_data)

    def delete_task(self, task_id):
        return self.send_request('delete', f'tasks/{task_id}')