---
title: "2. Maths4ML: Distances & Norms"
date: "2025-12-18"
description: "Measuring the gap - Rulers of ML"
tags: ["Maths for ML"]
pinned: false
---

The main point in ML is to find how similar 2 things are. To answer that the distance between these 2 things have to be found out.

Thus, to measure the world, ML algos use **norms**. 

## Standard Rulers

Norm or $L_p$ Norm ($ \| x\| $) is just a function that assigns a sytictly positive length or size to a vector. It tells how far away the point is from the origin $(0, 0)$.

### Euclidean Norm ($L_2$)

> It takes the most direct path possible.

- All the points which are exactly 1 unit away from a point when measured using this norm will form a perfect *Circle*.

- For a vector $x$ with $n$ dimensions :

$$ \| \mathbf{x}\|_2  = \sqrt{\sum_{i=1}^n x_i^2} = \sqrt{x_1^2 + x_2^2 + \cdots +x_n^2 }  $$


- This is just the pythagoras theorem.
- So the norm just tells the length of the vector from origin.

- For 2 vectors / points $x$ and $y$ the distance between them is calculated by *applying the norm to their differences*.

$$ \text{Distance} = \|x-y\|_2 = \sqrt{\sum_{i=1}^n (x_i - y_i)^2 } $$

- This distance is called **Euclidean Distance**.

>- Norm is a measure of length and Distance is a measure of Separation.
>- Distance is just the norm of the error.

So if $x$ and $y$ are almost the same, difference is tiny and the squared difference is even smaller.

But if $x$ is an outlier and far from $y$, the square becomes huge.

Thus, it punishes outliers heavily because of sqauring of errors. This makes $L_2$ norm great for things like MSE (Mean Squared Error) where making one huge mistake is more harmful than making many tiny ones.

---

### Manhattan Norm ($L_1$)

Unlike Euclidean norm where the shortest path is considered, in Manhattan Norm, the **movement is bounded to the grid axes**. Thus steps can only be taken along the x-axis and y-axis.

If all the points are considered which are 1 unit away from a center according to Mahattan Norm, the shape observed will be a **diamond** (square rotated by $ 45\degree$).

> It is called Manhattan Norm because Manhattan has a grid like structure and the distance between any 2 points is measured by moving in horizontal and vertical lines.

$$ \|x\|_1 = |x_1| + |x_2| + \cdots + |x_n| = \sum_{i=1}^n |x_i| $$

---

To understand the use of $L_1$ norm, we first need to understand **regularization**.

#### Reguarization

In a ML model, the best weights ($w_1, w_2$) are needed to minimie the error. This can be represented in terms of landscapes. High altitude means high error & low altitude mean low error. Thus, the lowest point in a pit is the *global minima* which will result in the best weights.

It is difficut to draw the 3D landscape/bowl on paper, so a 2D topographic map is used where all the points on same ring have equal error/altitude, as shown in the image below.


<img src="/static/images/M4ML1.png" alt="Loss-landscape" width="500" height="250">

---

In **Regularization** or leash 2 forces are fighting each other :

1. ***Error / Target*** : Go as close as possible to the global minima. It is surrounded by rings.
    - As we move away from the center the error gets higher.

2. ***Constraint / Leash*** : A leash tied around the origin which only allows to travel a distance of $c$ from the start.

<img src="/static/images/M4ML2.png" alt="Regularization" width="600" height="450">


- $\hat \beta$ is the target.

##### Lasso Regression 

- $ |\beta_1 | + |\beta_2| \le t $ creates a sharp-cornered diamond.

Since this is a leash, so the permisible part where the solution can exist is in the cyan diamond only. So the point where **red rings touch the coloured shape (here diamond)** is the final solution the model (Lasso Regression) chooses.

>- In $L_1$ norm almost always the ellipses will touch a corner of the diamond and since the corners are always on the main vertical or horizontal axis, a dimension will become 0.
>- This leads to sparsity, especially in higher dimensions.

Thus, $L_1$ norm is used for feature selection by dropping useless dimensions.

##### Ridge Regression

- $ \beta_1^2 + \beta_2^2 \le c $ creates a smooth circle.

Just like lasso regression, the red rings will touch the shape (here circle) where the solution will exist.

But it will not be on an axis but somewhere in between the vertical and horizaontal axis. The solution will have bit of both the feautures but the values will be small.

>- Lasso : Pulls the solution to axis, creating zeros.
>   - More useful in higher dimensions when there are lot more than 2 features.
>- Ridge : Preserves all features but keeps them small.

---

### Chebyshev Norm ($L_\infty$) 

Its movement is just like king in chess. It can move to any adjacent square and it will counted as just one move.

It will form a **square** if all the points which are at a distance of 1 unit from a center according to this norm are marked.

$$ \| \mathbf{x} \|_\infty = \max{(|x_1|, |x_2|, \cdots, |x_n|)} $$

It is just the absolute value of the largest element in the vector.

It can be used to minimize the maximum possible error.

## Context Aware Ruler

The above 3 rulers have a flaw that they treat all the dimensions equally. It doesn't consider the possibilty that the data features may have some correlation among them. 

Euclidean distance also doesn't consider for variance.

> Mahalanobis distance fixes this by measuring distance in terms of **statistical rarity** rather than physical length.

### Example :

A dataset of 2 features : *Salary* & *Age*.
- **Salary** : ranges from $20,000 to $200,000
- **Age** : ranges from 20 to 80

Lets say there are 2 people to be compared :
1. **Person A** : 50 years older than average.
2. **Person B** : Earns $50 more than average.

Euclidean distance will see the number 50 & say that these 2 people are equally far from the average. But commmon sense says that being 50 years older is a huge difference than earning just $50 more.

Thus, age & salary can't be measured with same ruler. They need to be measured relative to their spread / Standard Deviation.

### Mahalanobis Distance    

#### **First Adjustment** : <u>Scaling</u>

- Normalize the axes.
    - Shrinks the wide variables and stretches the narrow variables so they become comparable.

It stops asking *"How many units away is this point?"* & starts asking *"How many Standard Deviations away is this point?"*

In the example above, if SD of age = 10 years & SD of salary = $50,000, then :
- Being 50 years older is a distance of 5.
- Earning $50 more is a distance of 0.001.

#### **Second Adjustment** : <u>Rotating</u>

In datasets consisting of weights & heights which are highly correlated features, a point that is very tall & very light will be classified as :
- Euclidean dist : close to average because physical dist isn't huge.
- In reality, it is an **Outlier**.

$$ D_M(x) = \sqrt{(x-\mu)^T \Sigma^{-1}(x - \mu)} $$

- $(x-\mu)$ : Difference vector (how far is the point from average).
- $\Sigma$ : Covariance Matrix (holds the map of the data's shape (the variances & covariances)).
- $\Sigma^{-1}$ : Multiplying by the inverse is like dividing by the variance.
    - Divide the horizontal distance by the horizontal spread, and divide the vertical distance by the vertical spread.

---

## Minkowski Distance

$$ D_{A,B} = \left(\sum_{i=1}^n |A_i - B_i|^p \right)^{\frac{1}{p}} $$

- $p=1$ : Manhattan Distance
- $p=2$ : Euclidean Distance
- $p \rightarrow \infty$ : Chebyshev Distance

---

## The Geometry of Metrics

### Minkowski Distance: 

Use the slider to mutate the definition of distance, seeing how the "Unit Circle" transforms into a Diamond (Manhattan Norm) or a Square (Chebyshev Norm).

### Mahalanobis Distance: 

Toggle the variance mode to see how statistics changes geometry.

- Moving the mouse on the grid will dynamically calculate and compare the Statistical Distance vs. the Euclidean Distance.

- Every point on the ellipse at a time is equidistant with respect to Mahalanobis Distance.

<wasm-sim src="distance_shape">
            <script type="application/json">
            [
                {"id": "p", "label": "Minkowski p", "min": 1, "max": 20, "val": 2, "step": 0.1},
                {"id": "varianceMode", "label": "Enable Variance (Mahalanobis)", "type": "checkbox", "val": 0}
            ]
            </script>
        </wasm-sim>

---

With this, the notes on Norms and distances conclude.


<script src="static/js/wasm_engine.js"></script>