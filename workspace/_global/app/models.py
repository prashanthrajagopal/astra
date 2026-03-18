app/models.py
class Test:
    def __init__(self, id=None, name=None):
        self.id = id
        self.name = name

    def to_dict(self):
        return {'id': self.id, 'name': self.name}
