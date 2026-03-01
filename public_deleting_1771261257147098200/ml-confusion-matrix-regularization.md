---
title: "4. ML: Confusion Matrix, Bias-Variance & Regularization"
date: "2026-02-03"
description: "Model evaluation, bias-variance decomposition, and how regularization techniques constrain complexity."
tags: ["ML"]
pinned: false
---

## Model Evaluation : Confusion Matrix & Metrics

To optimize a model, its performance must be defined. In classification, raw accuracy is often insufficient, especially with imbalanced classes. The foundation of evaluation is the **Confusion Matrix**.

For a binary classifier, the predictions are categorized into 4 buckets based on the intersection of the predicted class and the actual truth values.

![Confusion Matrix](/static/images/Ml-10.png)

- **True Positive** (TP): Predicted (+) and Actual (+).
- **True Negative** (TN): Predicted (-) and Actual (-).
- **False Positive** (FP): Predicted (+) and Actual (-) (Type I Error).
- **False Negative** (FN): Predicted (-) and Actual (+) (Type II Error).

These 4 values are used to derive metrics to evaluate specific aspects of the model performance :

### 1. Accuracy

Ratio of correct prediction to the total pool.

$$ \text{Accuracy} = \frac{TP + TN}{TP+TN+FP+FN} $$

- Accuracy fails to distinguish between types of errors which is critical in unbalanced datasets.

### 2. Precision

The percentage of predicted positives that are actually positive.

$$\text{Precision} = \frac{TP}{TP + FP}$$

- High precision means low False Positives.
- Positive Predictive Value

### 3. Recall

The percentage of actual positives that were correctly identified.

$$\text{Recall} = \frac{TP}{TP + FN}$$

- **Sensitivity/True Positive Rate**
- High recall means low False Negatives.

### 4. Selectivity (Specificity) :

While Recall measures how well we find positives, **Selectivity measures how well we reject negatives**.

$$ \text{Selectivity} = \frac{TN}{TN + FP} $$

### 5. F1-Score

The harmonic mean of Precision and Recall. 

$$F_1 = \frac{2 \cdot \text{Precision} \cdot \text{Recall}}{\text{Precision} + \text{Recall}}$$

- It penalizes extreme values more than the arithmetic mean, ensuring a model is balanced.

### 6. Area Under ROC Curve (AUC-ROC)

> Pink: **Receiver Operating Characteristic (ROC)** curve plots the **Recall (TPR)** against the **False Positive Rate** $(1 - \text{Specificity})$ at various threshold settings.

The Area Under this Curve (AUC) represents the probability that the classifier will rank a randomly chosen positive instance higher than a randomly chosen negative one.

![AUC-ROC](/static/images/Ml-13.png)

> In case of more than 2 classes, compute Precision/Recall for each class independently, then average the scores. Treats all classes equally (good for checking performance on rare classes).

---

## Bias-Variance Trade-Off

One of the most fundamental problem is **Generalization** : Creating a hypothesis $h(x)$ that performs well on unseen data.

> The error of a model can be decomposed into 3 parts : **Bias**, **Variance** & **Irreducible Error**.

### Bias

It is the error caused by **oversimplifying assumptions** in the model. A high bias model will be too simple for the model and miss important patterns in the data. It will lead to **underfitting**.

Bias measures how far the average prediction of a model (over many different training sets) is from the true function.

### Variance

Variance is error caused by **too much sensitivity** to the training data. A high variance model will be too complex and fit the noise in the training data. It will lead to **overfitting**.

Variance measures how much the model’s predictions would change if it were trained on a different dataset.

<img src="/static/images/Ml-14.png" alt="Bias-Variance" width="500" height="450">


### Mathematical Derivation of MSE Decomposition

Assume a true relationship $y = f(x) + \epsilon$, where $\epsilon$ is noise. We estimate this with a model $\hat f(x)$.

The expected mean squared error (MSE) on an unseen sample $x$ is:

$$Error(x) = \mathbb{E}\left[ (y - \hat{f}(x))^2 \right]$$
$$= \mathbb{E}\left[ (f(x) + \epsilon - \hat{f}(x))^2 \right]$$
$$= \mathbb{E}\left[ (f(x) - \hat{f}(x))^2 \right] + \sigma^2 $$

Focusing on the estimation error $\mathbb{E}\left[ (f(x) - \hat{f}(x))^2 \right]$,

Let $\mathbb{E}[\hat{f}(x)]$ be the average prediction of our model over infinite training sets. We add and subtract this term :

$$\mathbb{E}\left[ (f(x) - \mathbb{E}[\hat{f}(x)] + \mathbb{E}[\hat{f}(x)] - \hat{f}(x))^2 \right]$$

Expanding this square $(a+b)^2 = a^2 + b^2 + 2ab$ :

- **Bias term** : $(f(x) - \mathbb{E}[\hat{f}(x)])^2$ is the squared Bias. 
  - It measures how far the average model is from the truth.
- **Variance term** : $\mathbb{E}\left[ (\mathbb{E}[\hat{f}(x)] - \hat{f}(x))^2 \right]$ is the Variance. 
  - It measures how much any single model fluctuates around the average model.
- **Cross term** : The cross term vanishes because $\mathbb{E}[\hat{f}(x) - \mathbb{E}[\hat{f}(x)]] = 0$.

Thus the Final Relation 

$$\text{Total Error} = \text{Bias}^2 + \text{Variance} + \text{Irreducible Error}$$

$$\text{Total Error} = \underbrace{(\mathbb{E}[\hat{f}(x)] - (f(x)))^2}_{\text{Bias}^2} + \underbrace{ \mathbb{E}\left[ (\mathbb{E}[\hat{f}(x)] - \hat{f}(x))^2 \right]}_{\text{Variance}} + \epsilon$$

Thus, the error is made up of bias and variance and it is important to find the right balance.

- **Low Complexity** : High Bias, Low Variance. The model is too rigid.
- **High Complexity** : Low Bias, High Variance. The model is too flexible and captures noise.
- The **Sweet Spot** : The goal is to find the complexity level where the sum of Bias$^2$ and Variance is minimized.

![Bias-Variance Tradeoff](/static/images/Ml-15.png)

---

- **Underfitting** : Occurs when a model is too simple to capture underlying patterns, performing poorly on both training and test data. 
- **Overfitting** : Occurs when a model is too complex and learns noise in the training data as if it were a real pattern. It leads to high accuracy on training data but poor performance on testing data.

![Underfitting & Overfitting](/static/images/Ml-16.png)

---

## Regularization

When the model is too complex, i.e. it is *overfitting*, the variance can be reduced by constraining the model weights. This is called regularization. It works by adding a penalty term to the Loss Function (Residual Sum of Squares [RSS]).

$$ \text{Total Cost} = \text{RSS} + \lambda \cdot (\text{Penalty Term}) $$

- $\lambda$ is the tuning parameter. 
- As $\lambda \to \infty$, coefficients shrink toward zero, reducing variance but increasing bias.

### Ridge Regression ($L_2$ Regularization)

Ridge adds the squared magnitude of coefficients as the penalty.

$$\boxed{ \hat{\beta}_{ridge} = \underset{\beta}{\text{argmin}} \left( \sum_{i=1}^n (y_i - \hat y_i )^2 + \lambda \sum_{j=1}^p \beta_j^2 \right)}$$

Expanding it :

$$\hat{\beta}_{ridge} = \underset{\beta}{\text{argmin}} \left( \sum_{i=1}^n (y_i - \beta_0 - \sum_{j=1}^p \beta_j x_{ij})^2 + \lambda \sum_{j=1}^p \beta_j^2 \right)$$

- **Shrinks coefficients toward zero but never exactly to zero**. It includes all features in the final model (no variable selection).

> Green: The constraint region $\beta_1^2 + \beta_2^2 \leq s$ is a circle. The RSS ellipses usually hit the circle at a non-axis point, keeping $\beta$ non-zero.

---

### Lasso Regression ($L_1$ Regularization)

Lasso (Least Absolute Shrinkage and Selection Operator) adds the absolute value of coefficients.

$$\boxed{ \hat{\beta}_{lasso} = \underset{\beta}{\text{argmin}} \left( \sum_{i=1}^n (y_i - \hat y_i )^2 + \lambda \sum_{j=1}^p |\beta_j| \right)}$$

Expanding it :

$$\hat{\beta}_{lasso} = \underset{\beta}{\text{argmin}} \left( \sum_{i=1}^n (y_i - \beta_0 - \sum_{j=1}^p \beta_j x_{ij})^2 + \lambda \sum_{j=1}^p |\beta_j| \right)$$

- **Can shrink coefficients exactly to zero, effectively performing feature selection**. It creates sparse models.

> Green: The constraint region $|\beta_1| + |\beta_2| \leq s$ is a diamond. The RSS ellipses often hit the "corners" of the diamond (the axes), forcing some coefficients to zero.

- It is not differentiable at 0.

![L1 vs L2 regularization](/static/images/Ml-17.png)


---

### Elastic Net

Elastic Net combines both $L_1$ and $L_2$ penalties to get the best of both worlds—feature selection (Lasso) and handling correlated features (Ridge).

$$\text{Objective} = \text{RSS} + \lambda_1 \sum |\beta_j| + \lambda_2 \sum \beta_j^2$$

- Use Case: Ideal when we have high-dimensional data $ (p \gt n) $ or highly correlated groups of features.

---

With this, the post on confusion matrix, metrics to evaluate the performance of the model, Bias-Variance Tradeoff and Regularization is completed.