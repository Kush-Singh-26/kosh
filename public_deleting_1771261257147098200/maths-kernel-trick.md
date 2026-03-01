---
title: "4. Maths4ML: Kernel Trick"
date: "2025-12-23"
description: "The Kernel Trick - Folding Space. Why struggle to bend the line when you can just fold the space?"
tags: ["Maths for ML"]
pinned: false
---

## Going Multidimensional

Linear Classifiers which draw a straight line to divide the space for classification fail when the data is not linearly separable. Below is an example of a *Non-linearly separable dataset*. 

<img src="/static/images/M4ML4.png" alt="Non-linearly separable dataset" width="300" height="250">

<details>
<summary>Click to see the code for creating the dataset.</summary>

```python
from sklearn.datasets import make_circles
import matplotlib.pyplot as plt

# Generate dataset
X, y = make_circles(
    n_samples=1000,
    noise=0.05,
    factor=0.5,
    random_state=42
)

# Plot
plt.scatter(X[:, 0], X[:, 1], c=y, cmap="bwr", s=10)
plt.axis("equal")
plt.show()
```

</details>

Clearly an algorithm like SVM or Logistic Regression would fail to classify the red and blue data samples here. 

> **Hyperplane** : A `n-1` dimensional boundary that divides a `n` dimensional space into 2 halves.

Since it is not possible to find a line (1D) that can divide this space (2D), we will have to go **multidimensional**. The core philosophy of kernel tricks is that : **Data that is tangled in low dimensions will likely be separable if we project it into high dimensions**.

> We go to high dimensions because it breaks the false intimacy of the low dimensions. It forces points that look close (but aren't) to move apart, allowing the similarity score to reflect their true nature.

## Transformation

### 2D Input Space

- The points are stuck in a $xy$ plane.
- Red dots are centered arounf $(0,0)$ and the blue balls are at a distance $\gt 5$.

### Mapping $\phi$

- Create a transformation $\phi(x)$.
- Add a new dimension $z$-axis whose coordinates will be based on distance from center.
- $z = x^2 + y^2$

### 3D Solution Space

- Now the data looks like a bowl or a parabola.
- Red dots having small $x,y$ are at the bottom of the bowl.
- Blue dots having large $x,y$ are at rim of the bowl.

> Now the bowl (3D Space) can be sliced with a **flat horizontal plane** at $z=c$. ($c$ being some constant).

## Why Kernel Trick?

The ML algos (eg. SVM) don't care about the coordinates ($xyz$ values). They only care about how similar 2 points A & B are.

The problem the above way is that when the data is to be lifted to not just 3D but to 1,000,000 dimensions for it to be perfectly separable then for each point (A & B), :
- Calculate 1,000,000 new coordinates for both point A & B.
- Multiply all of them to  caclculate their dot-product to see their similarity.

> **Result** : The computer crashes. It takes too much memory and time.

**Solution** : Use Kernel tricks to calculate the high-dimensional similarity from low-dimensional space.

## Kernel Trick

### Why similarity for creating boundaries (in SVMs)?

This is answered by **Representer Theorem**. 

The optimal solution (decision boundary) is simply a weighted sum of the similarities berween the new point and the training data.

$$ f(x) = \sum_{i=1}^n \alpha_iK(\mathbf{x}_i,\mathbf{x}) $$

- $K(\mathbf{x}_i,\mathbf{x})$ : Similarity / Kernel between a training point & new point.
- $\alpha_i$ : Weight / importance of that training point (learned by SVM).

So, SVM algorithm is basically a weighted voting system based on similarity.
- It looks at new data points and asks in complex high-dimension space, it the point more similar to red or blu support vector.
- This similarity check is done using kernel trick.

### Manual way

Let a transformation $\phi(x)$ be :

$$ \phi(x_1,x_2) = (x_1^2, \sqrt{2}x_1x_2, x_2^2) $$

Let 2 points be $\mathbf{x} = (2,3)$ and $\mathbf{y} = (4,5)$

Target is :

$$ \text{Target} = \phi(x) \cdot \phi(y) $$

- Map $\mathbf{x}$ : $(4, 6\sqrt{2},9)$
- Map $\mathbf{y}$ : $(16, 20\sqrt{2},25)$

Now calculate their similarity (dot product) in 3D space :

$$ (4 \times 16) + (6\sqrt{2} \times 20 \sqrt{2}) + (9 \times 25) = 64 + 240 + 225 = \mathbf{529} $$

### Kernel Way

> A Kernel is a formula applied to original numbers based on the mapping applied.

Here the Kernel Formula is : 

$$ K(x,y) = (x \cdot y)^2 $$

Dot product :

$$ (2 \times 4) + (3 \times 5) = 8 + 15 = 23 $$

Square it : $23^2 = \mathbf{529}$

Thus,

$$ K(x,y) = \phi(x) \cdot \phi(y) $$

Squaring the dot product in 2D  gives mathematically same answer to forst mapping and then taking the dot product way.

> Thus, the exact same result can be achieved in lower dimension itself with out too much computation.

This can also be written as :

$$k(\mathbf{x}_i, \mathbf{x}_j) = \langle \phi(\mathbf{x}_i), \phi(\mathbf{x}_j) \rangle$$

Where $\langle a, b \rangle$ means *inner product* which here is just the dot product.


## Safety Check (Mercer's Theorem)

Not every math function be a kernel. It must satisfy Mercer's Theorem :

>A symmetric function $k(\mathbf{x}, \mathbf{x}')$ is a valid kernel if and only if it is **Positive Semi-Definite (PSD)**.

### Postivive Semi-Definite (PSD)

A symmetric real matrix $A$ is Positive Semi-Definite if, for any non-zero vector $z$, the scalar result of $z^\top A z$ is non-negative ($ \ge 0$).$$z^\top A z = \sum_{i=1}^n \sum_{j=1}^n z_i z_j A_{ij} \ge 0$$

- Let there be a dataset $\{\mathbf{x}_1, \dots, \mathbf{x}_n\}$ and a kernel function $k$.
- Build a Gram Matrix (or Kernel Matrix) $\mathbf{K}$ where:

$$\mathbf{K}_{ij} = k(\mathbf{x}_i, \mathbf{x}_j)$$

- Mercer's Theorem states that for $k$ to be a valid kernel, this matrix $\mathbf{K}$ must be PSD for any set of inputs.
- Plugging $\mathbf{K}$ into the PSD definition using a coefficient vector $\mathbf{c}$ (instead of $z$):

$$\mathbf{c}^\top \mathbf{K} \mathbf{c} = \sum_{i=1}^n \sum_{j=1}^n c_i c_j k(\mathbf{x}_i, \mathbf{x}_j) \ge 0$$

This is actually measuring the **squared length or norm** of a vector.

Assuming a mapping $\phi(x)$ exists.

$$k(\mathbf{x}_i, \mathbf{x}_j) = \langle \phi(\mathbf{x}_i), \phi(\mathbf{x}_j) \rangle$$

Substituting this into the summation:

$$\sum_{i=1}^n \sum_{j=1}^n c_i c_j \langle \phi(\mathbf{x}_i), \phi(\mathbf{x}_j) \rangle$$

Because the inner product is bilinear (we can move sums inside), we can rewrite this as:

$$\left\langle \sum_{i=1}^n c_i \phi(\mathbf{x}_i), \sum_{j=1}^n c_j \phi(\mathbf{x}_j) \right\rangle$$

Let $\mathbf{V} = \sum_{i=1}^n c_i \phi(\mathbf{x}_i)$. This $\mathbf{V}$ is just a single vector in the feature space. The expression becomes:

$$\langle \mathbf{V}, \mathbf{V} \rangle = \| \mathbf{V} \|^2$$

> Thus the double summation $\sum \sum c_i c_j k(\mathbf{x}_i, \mathbf{x}_j)$ actually calculates the squared Euclidean norm (length) of some vector $\mathbf{V}$ in the high dimensional space.

As the double summation or the square of lengths must $\ge 0$. Thus :

- If a matrix is PSD, it guarantees that the high-dimensional space is valid and real.

- If it is Not PSD, it implies a broken geometry with imaginary distances, and the SVM math will collapse.

## Some common kernels & their mappings

### Linear Kernel

$$ k(\mathbf{x}, \mathbf{x'}) = \mathbf{x}^\top \mathbf{x'} $$

- Feature map : Identity mapping ($\phi(\mathbf{x}) = \mathbf{x}$)
- Used for linearly separable data

### Polynomial Kernel

$$ k(\mathbf{x}, \mathbf{x'}) = (\mathbf{x}^\top \mathbf{x'})^d $$

- Feature map : Contains all monomials upto degree $d$.

- Example : $d=2$ and $\mathbf{x} = [x_1, x_2]${top}$ :

$$ \phi(x) = [1, \sqrt{2}x_1, \sqrt{2}x_2, x_1^2, x_2^2, \sqrt{2}x_1x_2]^{\top} $$

Computing dot product of this 6D vector is computationally more expensive than computing $(\mathbf{x}^\top \mathbf{x'})^2$.

### Radial Basis Function (RBF)

This kernel projects the data into **infinite dimensions**.

$$ K(x,y) = e^{- \gamma \|x-y\|^2} $$

- $\|x-y\|^2$ : squared distance between points
- $-\gamma$ : It controls how strongly to penalize the distance
- $e^{\cdots}$ : Exponential function squashes the result between 0 & 1.

So points far apart will have $e^{-\text{huge}}\approx0$. Thus, no similarity.

---

## 2D-to-3D Lift

On left is the 2D input space which is being unsuccessfully trying to be separated via a straight line. On right is the 3D world where kernel (mappings) operates. 

When a lift is applied, data points (red & blue dots) rise based on ($z = x^2 + y^2$). Making the cut slider move will divide / classify the space. The shadow of the 3D plane can be observed as the circle on left side.

<wasm-sim src="kernel_trick">
            <script type="application/json">
            [
                {"id": "lift", "label": "Lift (Polynomial Degree)", "min": 0, "max": 2, "val": 0, "step": 0.05},
                {"id": "cut", "label": "Cut (Plane Height)", "min": -1, "max": 15, "val": -1, "step": 0.5}
            ]
            </script>
        </wasm-sim>

> The green sheet never bends. The problem of classification is not solved by curving the decision boundary, but by warping the space so that a flat boundary **looks** curved in 2D.

---

With this the post on mappings from low to high dimension (where data is easy to separate) to calculate similarity & making the process efficient by staying in the lower dimension using kernel tricks is complete!

<script src="static/js/wasm_engine.js"></script>