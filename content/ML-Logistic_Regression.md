---
title: "3. ML: Logistic Regression"
date: "2026-02-02"
description: "Probabilistic power of Logistic Regression: A deep dive into its linear roots and sigmoid derivation."
tags: ["ML"]
pinned: false
---

## Classification Problem

In supervised learning, when the target variable $y$ is discrete or categorical, the task is called **Classification**. It asks **which category?** unlike regression which asks **how much?**.
<br><br>
Given a dataset $\mathcal{D} = \{(\mathbf{x}^{(i)}, y^{(i)})\}_{i=1}^{m}$, where $\mathbf{x} \in \mathbb{R}^n$ are the features, we seek a function $f: \mathbb{R}^n \rightarrow \{C_1, C_2, ..., C_k\} $.

In *Binary Classification*, $y \in \{0,1\}$ where :

- $ 0 $ : Negative class (e.g., Benign tumor, Non-spam).
- $ 1 $ : Positive class (e.g., Malignant tumor, Spam).

---

## Logistic Regression is a **Generalized Linear Model (GLM)**

Logistic Regression is a regression model for probabilities. It predicts a continues probability value $\hat y \in [0,1] $, which is descretized using a threshold (0.5) to perform classification.

> Thus, it is a regression model adapted for classification tasks.

---

## Deriving Logistic Regression from Linear Regression

Linear model can be adapted for binary classification by mapping the output of the linear equation $z = \mathbf{w^Tx + b} $ to the interval $(0,1)$. This can be achieved by using the **Odds Ratio** and the **Logit function**.

### 1. Odds Ratio

The odds of an event occuring is the ratio of the probability of success $P$ to the probability of failure $ 1-P $.

$$ \text{Odds} = \frac{P(y=1|x)}{1 - P(y=1|x)} $$

- The range of the odds is $[0, \infty)$.

### 2. Logit (Log-Odds)

The output range of linear regression model is $(-\infty ,\infty )$. To map the range of the Odds ($[0, \infty)$) to this, take the natural log of the Odds :

$$ \text{logit}(P) = \ln\left(\frac{P}{1-P}\right) = \mathbf{w^T x + b} $$

This linear relation implies that the **log-odds** are linear w.r.t the input features $\mathbf{x}$.

### 3. Sigmoid Function

Isolate $P$ in the above equation as it is what we need to find out. For this, the logit function is inverted by raising both side to power of $e$:

$$ \frac{P}{1-P} = e^{\mathbf{w}^T\mathbf{x} + b} $$ 

$$ P = e^{\mathbf{w}^T\mathbf{x} + b} (1-P)  $$

$$ P = e^{\mathbf{w}^T\mathbf{x} + b} - e^{\mathbf{w}^T\mathbf{x} + b} P $$

$$ P (1 + e^{\mathbf{w}^T\mathbf{x} + b}) = e^{\mathbf{w}^T\mathbf{x} + b} $$

$$ P = \frac{e^{\mathbf{w}^T\mathbf{x} + b}}{1 + e^{\mathbf{w}^T\mathbf{x} + b}}  $$

Let $z = e^{\mathbf{w}^T\mathbf{x} + b}$. Then dividing the numerator and denominator by $e^{\mathbf{w}^T\mathbf{x} + b}$ :

$$ P = P(y=1|x) = \frac{1}{1+e^{-z}} = \sigma(z) $$

$\sigma(z)$ is called the **Sigmoid Function**.


![Sigmoid Mapping inputs to [0,1]](/static/images/Ml-9.png)

---

## Extending Binary Models to Multi-Class

As the name suggests, binary model is good for only 2 class problems. For more more than 2 classes a different approach is required.

### One vs All Approach

For $K$ classes, we train $K$ separate binary logistic regression classifiers. For each class $ i \in \{1, \dots, K\} $ , a model is trained to predict the probability that a data point belongs to that class $i$ versus all other classes. 
- Treat class $i$ as the "Positive" ($y=1$) class.
- Treat all other classes ($j \neq i$) as the "Negative" ($y=0$) class.

For prediction, we run all $K$ classifiers and choose the class that maximizes the probability.

### Softmax Regression / Multinomial Logistic Regression

It models the joint probability of all classes simultaneously. Instead of a single weight vector $\mathbf{w}$, we now have a weight matrix $W$ of shape $[K \times n]$ (where $n$ is the number of features). Each class $k$ has its own distinct weight vector $\mathbf{w}_k$.

For a given input $\mathbf{x}$ we compute a **score** or **logit** $z_k$ for each class $k$ 

$$z_k = \mathbf{w}_k^T \mathbf{x} + b_k$$

Or in vector notation :

$$\mathbf{z} = W\mathbf{x} + \mathbf{b}$$

#### Softmax Function

The sigmoid function maps a single value to $[0,1]$. The Softmax function generalizes this by mapping a vector of $K$ arbitrary real values (logits) to a probability distribution of $K$ values.The probability that input $\mathbf{x}$ belongs to class $k$ is given by:

$$ P(y=k| \mathbf{x}) = \text{softmax}(z)_k = \frac{e^{z_k}}{\sum_{j=1}^{K} e^{z_j}} $$

- It is like a max function but differentiable, hence *soft.*
- Sum of probabilities is 1.
- Each output value is in interval $(0,1)$.

---

## Loss Function : Maximum Likelihood Estimation (MLE)

### Why not use MSE?

MSE is used in Linear Regression because it creates a bowl-shaped convex curve. No matter where we start on the curve, via Gradient Descent we will eventually reach the absolute bottom (Global Minimum).

However, applying MSE in Logistic Regression which uses a sigmoid function (non-linear function) will result in a **non-convex**, wavy and complex graph.

> Maroon: **Convex Functions** : If a line segment between any 2 points of the function does not lie below the graph.

The non-convex curve has many *valleys* (local minima). If the algorithm starts in the wrong spot, it might get stuck in a shallow valley and think it has found the best solution when it hasn't. Hence, MSE is not used for Logistic Regression.

![MSE vs Log-loss](/static/images/Ml-7.png)


---

### Binary Cross-Entropy (BCE) Loss 

Due to failure of MSE, **BCE** is derived using **Maximum Likelihood Estimation** (MLE).

Instead of measuring the distance between the prediction and the target (like MSE), MLE asks a statistical question: **What parameters ($\mathbf{w}$) would maximize the probability of observing the data we actually have?**
<br><br>
**Assumption** : The target $y$ follows a *Bernoulli Distribution*, because the output can only be $ 0 $ or $ 1 $.

$$ P(y|\mathbf{x}) = \begin{cases} \hat{y} & \text{if } y=1 \\ 1-\hat{y} & \text{if } y=0 \end{cases} $$

- If the actual class is 1, we want the model to predict a high probability ($\hat{y}$).
- If the actual class is 0, we want the model to predict a low probability (which means $ 1-\hat{y} $ is high).

This can be compacted to :

$$ P(y|x) = \hat y^y (1- \hat y)^{(1-y)} $$

Assuming $m$ independent training examples, the Likelihood $\mathcal{L}(\mathbf{w})$ of the parameter $\mathbf{w}$ is the product of the probabilities of the observed data :

$$\mathcal{L}(\mathbf{w}) = \prod_{i=1}^{m} \left( \hat{y}^{(i)} \right)^{y^{(i)}} \left( 1-\hat{y}^{(i)} \right)^{(1-y^{(i)})}$$

> To simplify differentiation and avoid numerical overflow, we *maximize* the **Log-Likelihood** $l(\mathbf{w})$. This makes the Product $(\Pi)$ to Summation $(\sum)$. 

Taking **natural log $(\ln)$** :

$$l(\mathbf{w}) = \ln \mathcal{L}(\mathbf{w}) = \sum_{i=1}^{m} \left[ y^{(i)} \ln(\hat{y}^{(i)}) + (1-y^{(i)}) \ln(1-\hat{y}^{(i)}) \right]$$

But optimization algos are designed to minimize the error, not maximizze the likelihood. Thus, this equation is inverted by adding a <span class="red">**negative**</span> sign.

Thus, the final **Binary Cross-Entropy Loss** or the **Negative Log-Loss** function is :

$$ \boxed{ \ln \mathcal{L}(\mathbf{w}) = - \sum_{i=1}^{m} \left[ y^{(i)} \ln(\hat{y}^{(i)}) + (1-y^{(i)}) \ln(1-\hat{y}^{(i)}) \right]} $$

Thus, the average BCE loss is :

$$ \boxed{J(\mathbf{w}) = \mathcal{L}_{BCE} = - \frac{1}{m} \sum_{i=1}^m \left[ y^{(i)} \ln (\sigma(\mathbf{w}^T \mathbf{x}^{(i)} +b )) + (1- y^{(i)}) \ln (1- \sigma(\mathbf{w}^T \mathbf{x}^{(i)} +b))  \right]} $$

- This is a convex function and thus gradient descent will converge to the global minima.

---

## Gradient Descent Derivation

> Green: **Goal** : Minimize $J(\mathbf{w})$. For this, the gradient $\frac{\partial J}{\partial w_j} $ is needed.

### 1. Derivative of Sigmoid Function

$$ \frac{d}{dz} \sigma(z) = \sigma(z) (1- \sigma(z)) $$

### 2. Derivative of the loss (Chain Rule)

Using only a single example :

$$ L = -[y \ln (\hat y) + (1-y) ln (1 - \hat y)] \quad , \, \hat y = \sigma(z) $$

Using the chain rule :

$$ \frac{\partial L}{\partial w_j} = \frac{\partial L}{\partial \hat y} \cdot \frac{\partial \hat y}{\partial z} \cdot \frac{\partial z}{\partial w_j}  $$

- Partial of Loss w.r.t Prediction :
    
$$\frac{\partial L}{\partial \hat{y}} = -\left( \frac{y}{\hat{y}} - \frac{1-y}{1-\hat{y}} \right) = \frac{\hat{y} - y}{\hat{y}(1-\hat{y})}$$

- Partial of Prediction w.r.t Logit : $\hat{y} = \sigma(z) = \frac{1}{1 + e^{-z}}$
  
$$\frac{\partial \hat{y}}{\partial z} = \hat{y}(1-\hat{y})$$

- Partial of Logit w.r.t Weight : $z = \sum w_j x_j + b$
 
$$\frac{\partial z}{\partial w_j} = x_j$$

### 3.  Combining Terms 

$$ \frac{\partial L }{\partial w_j} = \frac{\hat{y} - y}{\hat{y}(1-\hat{y})} \cdot \hat{y}(1-\hat{y}) \cdot x_j  $$

The term $\hat{y}(1-\hat{y})$ cancels out, leaving :

$$\frac{\partial L}{\partial w_j} = (\hat{y} - y)x_j$$

---

### Batch Gradient Descent

Update weights using the average gradient over the entire dataset $m$:

$$w_j := w_j - \alpha \frac{1}{m} \sum_{i=1}^{m} (\hat{y}^{(i)} - y^{(i)}) x_j^{(i)}$$

### Stochastic Gradient Descent

Update the weights for each training example $(x^{(i)}, y^{(i)})$ individually:

$$ w_j := w_j - \alpha (\hat y^{(i)} - y^{(i)})x_j^{(i)}  $$

> Pink: This rule is identical in form to the [Linear Regression update rule](/ML-Linear_Regression.md) but the definition of $\hat{y}$ has changed from linear ($\mathbf{w}^T\mathbf{x}$) to sigmoid ($\sigma(\mathbf{w}^T\mathbf{x})$).

---

## Binary Logistic Regression is a linear classifier.

To prove that a classifier is **linear**, we must show that the decision boundary separating the classes is a linear function of the input features $\mathbf{x}$ (i.e., a line, plane, or hyperplane).

In logistic regression, the probability prediction is given by the sigmoid function applied to a linear combination of inputs.

$$ P(y=1|x) = \sigma(z) = frac{1}{1+ e^{-z}} $$

$z$ (logits) is a linear function of the weights and features :

$$z = \mathbf{w}^T\mathbf{x} + b = w_1x_1 + w_2x_2 + \dots + w_n x_n + b$$

An instance is classified into the positive class ($y=1$) if the predicted probability is greater than or equal to a threshold, typically $ 0.5 $.

$$\text{Predict } y=1 \iff P(y=1|\mathbf{x}) \ge 0.5$$

$$\frac{1}{1 + e^{-z}} \ge 0.5$$
$$ 1 \ge 0.5 (1+ e^{-z}) $$
$$ 2 \ge 1+ e^{-z} $$
$$ 1 \ge e^{-z} $$

Taking the natural log ($\ln$) of both sides

$$\ln(1) \ge -z$$
$$0 \ge -z \implies z \ge 0$$
$$\mathbf{w}^T\mathbf{x} + b \ge 0$$

The decision boundary is the exact point where the classifier is uncertain (probability $= 0.5$), which corresponds to the equation:

$$\mathbf{w}^T\mathbf{x} + b = 0$$

- This represents a hyperplane (line in 2D, plane in 3D).

> Since the boundary that separates the two classes in the feature space is linear, Logistic Regression is, by definition, a linear classifier.

- While the decision boundary is linear, the relationship between the input features and the predicted probability is non-linear (S-shaped or sigmoidal). 
- The "linear" label strictly refers to the shape of the separation boundary in space, not the probability curve.

![Linear Separation in Feature Space](/static/images/Ml-8.png)

---

With this, the post on binary classifiers, logistic regression, Binary Cross Entropy Loss / negative log-loss comes to an end.