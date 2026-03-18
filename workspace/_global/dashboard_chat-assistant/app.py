from flask import Flask, jsonify, request
from agent import AstraAgent

app = Flask(__name__)

agent = AstraAgent()

@app.route('/goals', methods=['GET'])
def get_goals():
    goals = agent.get_goals()
    return jsonify(goals)

@app.route('/goals', methods=['POST'])
def create_goal():
    data = request.get_json()
    goal = agent.create_goal(data)
    return jsonify({'goal_id': goal})

@app.route('/agents', methods=['GET'])
def get_agents():
    agents = agent.get_agents()
    return jsonify(agents)

@app.route('/agents/<agent_id>/goals', methods=['GET'])
def get_agent_goals(agent_id):
    goals = agent.get_agent_goals(agent_id)
    return jsonify(goals)

@app.route('/tasks', methods=['GET'])
def get_tasks():
    tasks = agent.get_tasks()
    return jsonify(tasks)

if __name__ == '__main__':
    app.run(debug=True)