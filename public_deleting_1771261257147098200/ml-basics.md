---
title: "1. ML: Basic Fundamentals"
date: "2026-01-30"
description: "Building Blocks of Machine Learning"
tags: ["ML"]
pinned: false
---

## Dataset Representation

Tabular datasets consists of rows and columns :

- **Rows** : Also called *data points*, *samples*, *observations*, *instances*, *patterns*.
    - Each row reprsents a single observation.

- **Columns** : Also called *variables*, *characteristics*, *features*, *attribute*.
    - Each columns represents a measurable property or attribute of that observation.

| | H | W |
|---|---|---|---|
| $p_1$ |130|55|
|$p_2$|140|65|
|$\vdots$|$\vdots$|$\vdots$|
|$p_n$|160|75|

To perform statistical analysis, the datasets is viewed as **samples drawn from a probability distribution**.

- Each **feature** (column) is treated as **Random Variable** ($X$).
    - A RV is a function that maps outcomes of a random phenomenon to numerical values.
    - Thus, $X_{height}$ is a rv describing the distribution of heights feature in the population.

- A single **row** containing $d$ features is a **random vector**.
    - If the dataset has features $X_1$, $X_2$, $\dots$, $X_d$, then a single observation is a vector $\mathbf{x} = [x_1, x_2, \dots, x_d]^T$.

- Thus, the entire dataset is a collection of $n$ observed random vectors :

$$ \mathcal{D} = \{ \mathbf{x}_1, \mathbf{x}_2, \dots, \mathbf{x}_n \} = \text{Dataset} $$

$$ \mathbf{x}_i = [x_{i1}, x_{i2}, \dots, x_{id}]^T \text{ random vector} $$

Thus, the full table can be visualized like :

$$\mathbf{X} = \begin{pmatrix} \rule[0.5ex]{1.5em}{0.5pt} & \mathbf{x}_1^T & \rule[0.5ex]{1.5em}{0.5pt} \\ \rule[0.5ex]{1.5em}{0.5pt} & \mathbf{x}_2^T & \rule[0.5ex]{1.5em}{0.5pt} \\ & \vdots & \\ \rule[0.5ex]{1.5em}{0.5pt} & \mathbf{x}_n^T & \rule[0.5ex]{1.5em}{0.5pt} \end{pmatrix} = \begin{pmatrix} x_{11} & x_{12} & \cdots & x_{1d} \\ x_{21} & x_{22} & \cdots & x_{2d} \\ \vdots & \vdots & \ddots & \vdots \\ x_{n1} & x_{n2} & \cdots & x_{nd} \end{pmatrix}$$

---

## Measures of Central Tendencies

### Moment 1 

#### 1. Mean

$$ \mu = \frac{1}{n} \sum_{i=1}^n x_i = \bar{x} $$

- For a dataset :

$$X = \begin{bmatrix} 
x_{11} & x_{12} & x_{13} & \dots & x_{1d} \\
x_{21} & x_{22} & x_{23} & \dots & x_{2d} \\
\vdots & \vdots & \vdots & \ddots & \vdots \\
\underbrace{x_{n1}}_{\bar{x}_1} & \underbrace{x_{n2}}_{\bar{x}_2} & \underbrace{x_{n3}}_{\bar{x}_3} & \dots & \underbrace{x_{nd}}_{\bar{x}_d}
\end{bmatrix}$$

Where :
- $\bar{x}_j = \frac{1}{n} \sum_{i=1}^{n} x_{ij}$

And thus the resulting mean vector $\mu$ is a collection of these individual feature means :

$$\mu = \bar{X} = \begin{bmatrix} \bar{x}_1 & \bar{x}_2 & \bar{x}_3 & \dots & \bar{x}_d \end{bmatrix}^T$$

#### 2. Median

First, sort the data in ascending order.

If odd no. of values :

$$ \text{Median} = x_{\frac{n+1}{2}} $$

If even no. of values :

$$ \text{Median} = \frac{1}{2} [x_{\frac{n}{2}} + x_{\frac{n+1}{2}}] $$

> Red: When outliers are present in the dataset, it is better to use **median**.

### Moment 2 (Measures of Dispersion)

#### 3. Variance

Measures how far are data points spread out from the mean.

$$ \sigma^2 = \frac{1}{n-1} \sum_{i=1}^n (x_i - \mu)^2 $$

- It heavily weight the outliers because it squares the difference.

#### 4. Standard Deviation

$$ \sigma = \sqrt{\frac{1}{n-1} \sum_{i=1}^n (x_i - \mu)^2} $$

- Square root of variance.
- It measures the average distance of data points from the mean.

#### 5. Range

$$ \text{Range} = x_{max} - x_{min} $$

### Moment 3 / **Skewness**

Measures the asymmetry / symmetry of the distribution around the mean.

$$ \text{Skewness} = \frac{1}{n} \sum{\left(\frac{x_i - \mu}{\sigma}\right)^3} $$

- **Positive Skew** : Tail extends to the right (right skewed).
- **Negative Skew** : Tail extends to the left (left skewed).
- **Zero Skew** : Perfectly symmetrical (like a standard Normal distribution).

![Skewness](/static/images/Ml-1.png)

### Moment 4 / **Kurtosis**

It defines the shape in terms of peak (sharpness) and tail (heaviness).

$$ \text{Kurtosis} = \frac{1}{n} \sum{\left(\frac{x_i - \mu}{\sigma}\right)^4} $$

> Green: In denominator, **Bessels correction** (use of $n-1$) will be done when a **sample of the population** is considered. Otherwise, when the whole population is used, use $n$.

---

## Box Plot

It is a standard way of displaying the distribution of data based on **5-number summary**.

1. **Minimum** ($Q_0$) : lowest data point excluding any outliers.
2. **First Quartile** ($Q_1$ / 25th Percentile) : The value below which 25% of the data falls. The bottom of the box.
3. **Median** ($Q_2$ / 50th Percentile) : The middle value of the dataset. The line inside the box.
4.  **Third Quartile** ($Q_3$ / 75th Percentile) : The value below which 75% of the data falls. The top of the box.
5. **Maximum** ($Q_4$) : The highest data point excluding any outliers.

---

- **Interquartile Range (IQR)** : The height of the box ($Q_3 - Q_1$). It represents the middle 50% of the data.

- **Whiskers** : Lines extending from the box indicating variability outside the upper and lower quartiles. 
    - Set to $Q_1 - 1.5 \times IQR$ and $Q_3 + 1.5 \times IQR$

- **Outliers** : Individual points plotted beyond the whiskers.

![Box Plot](/static/images/Ml-3.png)

---

## Covariance and Correlation

When analysing 2 features or Random Variables ($X$ and $Y$), it is better to look at their joint variability.

### Covariance

$$ Cov(X,Y) = \frac{1}{n-1} \sum{[(X-\bar{X})(Y-\bar{Y})]}$$

$$ = \, E[(X-\mu_X)(Y-\mu_Y)] $$

It measures the direction of the linear relationship between variables.

- **Positive Covariance** : As $X$ increases, $Y$ tends to increase.
- **Negative Covariance** : As $X$ increases, $Y$ tends to decrease.
- **Zero Covariance** : No linear relationship between the 2 RVs.

![Covariance](/static/images/Ml-2.png)

---

### Correlation

It is the normalized version of covariance. It measures both the strength and direction of linear relationship.

$$ \rho_{X,Y} = \frac{Cov(X,Y)}{\sigma_X \sigma_Y} $$

- It is the Pearson Correlation Coefficient.
- It will always be between 1 and -1.

---

> Orange: Covariance of a RV '$X$' with itself will be $(E[X-E[X])(E[X-E[X]]) = E[(X-E[X])^2] = \sigma_X^2 $.   
> Thus, $Cov(X,X) = Var(X)$.

### Covariance Matrix

For a random vector with $d$ features, the relation between all features can be summarized using the *Covariance Matrix* $(\sum)$.
- It is a $d \times d$ matrix.


$$\Sigma = \begin{pmatrix}
Var(X_1) & Cov(X_1, X_2) & \cdots & Cov(X_1, X_d) \\
Cov(X_2, X_1) & Var(X_2) & \cdots & Cov(X_2, X_d) \\
\vdots & \vdots & \ddots & \vdots \\
Cov(X_d, X_1) & Cov(X_d, X_2) & \cdots & Var(X_d)
\end{pmatrix}$$

- **Diagonal elements** : Variance of individual terms.
- **Off-Diagonal elements** : Covariances between feature pairs.
  - $Cov(X,Y) = Cov(Y,X)$ , this means that the matrix is symmetric.

> Green: If one feature is a perfect linear combination of other features, then there is redundancy in the information, and the covariance matrix is singular (i.e., its rank is less than the number of features).

---

### Correlation Matrix

While the Covariance Matrix tells the direction of the relationship and the spread, the Correlation Matrix provides a normalized score of the relationship strength, making it easier to compare features with different units (e.g., comparing "Height in cm" vs. "Weight in kg").

$$\mathbf{R} = 
\begin{pmatrix}
1 & \rho_{12} & \cdots & \rho_{1d} \\
\rho_{21} & 1 & \cdots & \rho_{2d} \\
\vdots & \vdots & \ddots & \vdots \\
\rho_{d1} & \rho_{d2} & \cdots & 1
\end{pmatrix}$$

- $\rho_{ij}$ : It is the Pearson Coefficient.
  - $\rho_{ij} = \frac{Cov(X_i,X_j)}{\sigma_{X_i} \sigma_{X_j}} $
- It is between $[1,-1]$.
- It is also a symmetric matrix.

---

## Types of Machine Learnings

1. **Supervised Learning**
- Model learns from labelled data. For every input, the correct output is already known. The goal is for the algorithm to learn the mapping function from the input to the output.
- Eg. : Linear Regression, Logistic Regression, SVM, Decision Tree, KNN, Neural Networks, etc.
- Use cases : Email spam filtering, Medical diagnosis, Credit Scoring, etc.

2. **Unsupervised Learning**
- The model works with unlabelled data and finds hidden patterns.
- Eg. Clustering, Dimensionality Reduction (PCA)
- Use cases : Customer segmentation, Anomaly detection, Association discovery

3. **Semi-Supervised Learning**
- The model is trained on a small amount of labeled data and a large amount of unlabeled data.
- eg. Self-training models, Transformers
- Image classification when labelling data is expensive.

4. **Reinforcement Learning**
- An *agent* learns to make decisions by performing actions in an environment to achieve a goal. It receives rewards for good actions and penalties for bad ones.
- Examples : Policy Gradient Methods
- Use cases : Robotics, Self-driving cars, Game plating (chess, go)

---

## Supervised Learning

1. Start with a labelled dataset where input (features) and outputs (labels) are known.
2. Split the dataset into `train-test`.
  - *Training Set* : Used to build and tune models.
    - It is split into 2 parts :
      - Train split
      - Validation split
  - *Test Set* : Held out and never used during training or model selection. It is only used at the very end to estimate real-world performance.
3. Using the training set, **multiple candidate models** are fitted based on different hyperparameters or algos.
4. Validation set is used to evaluate these models during development.
5. Based on the validation performance the best model is selected (highest validation accuracy, lowest loss).
6. The selected model becomes the final trained model.
7. It is evaluated on the test set, producing an unbiased estimate of the performance.


```d2
direction: down

A: "Labeled Dataset"

A -> B: " "
A -> C: " "

B: "Training Set"
C: "Test Set"

G: {
  label: "Model Development"
  direction: down
  style.stroke: "#2ecc71"
  style.stroke-width: 3

  B1: "Training Set"
  B2: "Validation Set"

  B1 -> D
  B2 -> E
  D -> E

  D: "Learned models"
  E: "Select model"
}

B -> G.B1
B -> G.B2

G.E -> F
C -> F

F: "Supervised Learned Model"
F -> H

H: "Accuracy Estimate"
```

---

With this, the basics are over.