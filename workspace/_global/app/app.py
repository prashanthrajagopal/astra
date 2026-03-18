app/app.py
from flask import Flask, request, jsonify
from app.db import db
from app.models import Test

app = Flask(__name__)

@app.route('/test', methods=['GET'])
def get_test():
    tests = Test.query.all()
    return jsonify([test.to_dict() for test in tests])

@app.route('/test', methods=['POST'])
def create_test():
    data = request.get_json()
    test = Test(**data)
    db.session.add(test)
    db.session.commit()
    return jsonify(test.to_dict()), 201

if __name__ == '__main__':
    app.run(debug=True)
