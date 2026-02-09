---
title: "1. Neural Nets: Overview"
description: "Overview of Neural Networks"
tags: ["Neural-Net"]
date: 2025-03-13
pinned: false
---

# Neural Networks

- A **neural network** (or neural net) is a machine learning model inspired by the human brain. It consists of layers of interconnected nodes (neurons) that process and transform input data to make predictions or classifications. 

## Architecture of a simple Neural Net or Multi-Layer Perceptron (MLP)
- It consists of 3 layers   
<img src="/static/images/NN1.png" alt="Image" width="500" height="350">

### Input Layer
- This layer receives raw input features.
- Each node represents / corresponds to a feature in the dataset
### Hidden Layers
- Performs computation and extracts patterns from the input data
- There can be many hidden layers, depending on the task's complexity.

### Output Layer
- Produces the model's predictions.
- Softmax activation for classification task
- Linear activation for Regression task

## Neurons, Weights and Biases
- Each layer in a Neural Net (NN) consists of **neurons**
- Each neuron applies a weighted sum of its inputs followed by an activation function.

- $$ z = W_1x_1 + W_2x_2 + ... + W_nx_n + b $$
    - x<sub>1</sub>, .., x<sub>n</sub> are the input features.
    - W<sub>1</sub>, .., W<sub>n</sub> are the weights associated with each input
    - b : bias 
    - z : weighted sum which is passed through an activation

## Forward Propagation
- Data moves from the input layer through the hidden layers to the output layer.
- Each layer processes the input using weights, biases, and activation functions ans passes the output to the next layer.

## Loss Function and Backpropagation
- Loss function is used to measure how far away the predicted value from the actual value / ground truth.
- Examples of loss funtions :
    - **Mean Squared Error (MSE**) : for regression tasks.
    - **Cross-Entropy Loss** : for classification tasks.

- **Backpropagation** is an algorithm used to update the weight of a neural net.
- It computes the gradient of the loss function with respect to each weight using the _chain rule (of calculus)_ and updates the weights using an optimization algorithm (eg. Stochastic Grdient Descent (SGD) or Adam).

- Covered _backpropgation_ in depth [here](https://github.com/Kush-Singh-26/Micrograd).

---

- Neurons apply weighted sums and activation functions to learn patterns.

- Forward propagation moves data through the network 

- backpropagation updates weights to minimize error.

- The choice of architecture, activation functions, and optimization techniques affects the performance of a neural network