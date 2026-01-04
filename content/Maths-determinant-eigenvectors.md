---
title: "6. Maths4ML: Determinants & Eigenvectors"
date: "2025-12-27"
description: "Finding the stillness inside the transformation."
tags: ["Maths for ML"]
pinned: false
---

## Matrices are opaque

As established in last blog on [matrices](./Maths-matrices.md), they are machines which transforms a vector inputed to them. But any random matrix doesn't reveal what it is doing to data. To find out the *DNA* of the matrix, these 2 tools are needed :

1. **Determinant** : tells how much space is expanding or shrinking.
2. **Eigen Vectors** : tells along which line the stretching is happening.

## Determinant (Scaling Factor)

To get the geometric intuition, let a unit square be sitting at the origin $(0,0)$. Area of the square is 1. When a matrix (linear transformation) is applied to this space, the grid line warp. The square will stretch into a long rectangle, skew into a diamond or even shrink into dot.

> Determinant is the new area of the square.

- **Det = 2** : Space is stretching. Area of square doubles.
- **Det = 0.5** : Space is contracting. Area is halved.
- **Det = 1** : Shape might change. Area remains the same.

### Singular Matrix

A matrix whose **determinant = 0**. This means the area of square becomes 0. This means the square is squashed completely flat into a single line or even a point.  

> This matrix which collapses the space is called **Singular Matrix**.

This matrix *destroys information*. When the square is squashed into a line, all information about how the original square looked like is lost. **This process is irreversible**.

> Thus, singular matrix (matrix with det = 0) is **non-invertible**.

---

For a $2 \times 2$ matrix $A$ :

$$ \begin{bmatrix} a&b \\ c&d \end{bmatrix} $$

The determinant is : 

$$ \text{det}(A) = ad - bc $$

> The determinant is the factor by which the linear transformation scales area.

## Derivation of Determinant

$$ A = \begin{bmatrix} \color{green}{a}&\color{red}{b} \\ \color{green}{c}&\color{red}{d} \end{bmatrix} $$

This transforms the basis vectors into 2 new vectors :
1. <span class = "green">Vector 1</span> : $v_1 = \color{green}{(a,c)}$
    - Moves $a$ steps right, $c$ steps up
1. <span class = "red">Vector 2</span> : $v_2 = \color{red}{(b,d)}$
    - Moves $b$ steps right, $d$ steps up

> These 2 vectors will form a parallelogram. Det is the area of this parallelogram.

![2D matrix determinant derivation](/static/images/M4ML6.png)

To find the area of the rectangle, subtract the area of the big box and the empty spaces surrounding the parallelogram.

- As can be observed from the figure, area of big box = $(a+b)(c+d)$.
- The waste area consist of 2 of each of the blue & yellow triangle and the 2 pink rectangles.
    - Their combined area = $ (a\times c) + (b \times d) + (2bc) $ 

Thus, 

$$ \text{det}(A) = (a+b)(c+d) - (ac + bd + 2bc) $$
$$ = (ab + ad + bc + bd) - ac - bd - 2bc $$
$$ = ad + bc - 2bc $$

$$ \fbox{\text{det}(A) = ad - bc}$$

### Why determinant is the scaling factor?

The term $ad$ represents scaling along the main axes based on the original basis vectors.

The term $bc$ represents the twist or shears that interfere with the area.

> So subtraction of the twist from stretch gives the true scaling factor.

---

#### Negative Determinant

It signifies that the universe has been *flipped over*. This means that the vectors are arranged in a **clockwise** manner.

$$ A = \begin{bmatrix} 2&0 \\ 0&2 \end{bmatrix} \quad \bigg| \quad A '= \begin{bmatrix} 0&2 \\ 2&0 \end{bmatrix} $$

- $A$ is the standard orientation (counter-clockwise) ans hence its determinant is positive.
- $A'$ is the swapped orientation (clockwise) ans hence its determinant is negative.

> Thus, negative determinant just shows that the orientation has flipped.

---

## Deteminant Drag

Drag the red and green basis vectors to change the matrix value. These vectors represent the columns of the transformation matrix.

- **Blue Square** : Positive determinant (normal orientation)
- **Purple Square** : Negative determinant (flipped orientation)
- **Collapsed Line** : When determinant ≈ 0, the square flattens completely

There are also pre-existing presets for the transformation matrix.

<wasm-sim src="determinant_drag">
        <script type="application/json">
        [
            {"id": "a", "label": "Matrix a (top-left)", "min": -3, "max": 3, "val": 1, "step": 0.1},
            {"id": "b", "label": "Matrix b (top-right)", "min": -3, "max": 3, "val": 0, "step": 0.1},
            {"id": "c", "label": "Matrix c (bottom-left)", "min": -3, "max": 3, "val": 0, "step": 0.1},
            {"id": "d", "label": "Matrix d (bottom-right)", "min": -3, "max": 3, "val": 1, "step": 0.1},
            {"id": "reset", "label": "Reset to Identity", "type": "button"},
            {"id": "shear", "label": "Shear Preset", "type": "button"},
            {"id": "singular", "label": "Singular Preset", "type": "button"}
        ]
        </script>
</wasm-sim>

---

## Eigen Vectors (Stubborn Vectors)

Let there is a paper or a grid with some vectors (arrows) on it, and the paper is stretched from left and right side horizontally.

- A vector pointing $45\degree$ will tilt and will be pulled horizontally.
- The vector which was pointing **perfrectly horizontally** (eg. $\hat{i}$) will not tilt. It will just get longer.
- The vector which was pointing **perfrectly vertically** (eg. $\hat{j}$) will also not tilt.

> Eignen vectors are these **stubborn arrows**. These vectors stay on the same path even when other vectors get knocked off their path when a matrix transformation is applied. They don't change direction but only get scaled.

- **Eigenvector ($v$)** : The vector that refuses to rotate.
- **Eigenvalue ($\lambda$)** : The number describing how much the vector is stretched.

> Eigenvectors is like the skeleton of the matrix. It reveals the principle axis along which the transformation is acting.

---

$$ Av = \lambda v $$

- $Av$ : Take the vector $v$ and transform it with matrix $A$.
- $\lambda v$ : Take the original vector $v$ and scale it by $\lambda$.
- The transformation didn't change the direction. It acted exactly like scalar multiplication.

---

## Characteristic Equation

To find the eigen vectors, rearrange the above equation :

$$ Av - \lambda v = 0 $$

Factoring out $v$ :

$$ (A - \lambda I)v = 0 $$

If the matrix ($A - \lambda I$) is ivertible, then the only solution will be $v=0$. For a non-zero vector $v$ that solves this equation, the matrix ($A - \lambda I$) will have to squash the vector-space (send the non-zero vector to zero). This means the **determinant of the matrix must be 0**.

$$\det(A - \lambda I) = 0$$

<details>
<summary> Solved Example of finding eigen vectors and eigen values</summary>

$$A = \begin{bmatrix} 4 & 1 \\ 2 & 3 \end{bmatrix}$$

  Solving the Characteristic equation using the above matrix : $\det(A - \lambda I) = 0$.

$$\det \left( \begin{bmatrix} 4 & 1 \\ 2 & 3 \end{bmatrix} - \begin{bmatrix} \lambda & 0 \\ 0 & \lambda \end{bmatrix} \right) = 0$$

$$\det \begin{bmatrix} 4-\lambda & 1 \\ 2 & 3-\lambda \end{bmatrix} = 0$$

$$(4-\lambda)(3-\lambda) - (1)(2) = 0$$

$$\lambda^2 - 7\lambda + 10 = 0$$

$$(\lambda - 5)(\lambda - 2) = 0$$

So the 2 eigenvalues are : $\lambda_1 = 5$ and $\lambda_2 = 2$.

Solving the linear system $(A - \lambda I)u = 0$ for the vector $v = \begin{bmatrix} x \\ y \end{bmatrix}$ will give the eigen vectors.

- $\lambda = 5$ : It will give $x=y$. So any vector where $x$ equals $y$ is an eigenvector. 
    - **Eigenvector 1** : $\vec{v}_1 = \begin{bmatrix} 1 \\ 1 \end{bmatrix}$

- $\lambda = 2$ : It will give $y=-2x$. 
    - **Eigenvector 2** : $\vec{v}_2 = \begin{bmatrix} 1 \\ -2 \end{bmatrix}$
</details>

---

### Relationship between trace & eigenvalues and determinant & eigenvalues

- **Trace** : sum of the top-left to bottom-right diagonal.

$$\lambda_1 + \lambda_2 = \text{Trace}(A)$$

$$\lambda_1 \cdot \lambda_2 = \det(A)$$

---

## EigenVector Hunt

- <span class="red">Red Arrow</span> : input vecetor
- <span class="yellow">Yellow Arrow</span> : transformed vecetor
- <span class="green">Green Arrow</span> : When the input and transformed vector align, that is the eigen vector.

There are 3 presets for the transformation matrix.

Drag the input vector to find the *eigenvector*.

<wasm-sim src="eigen_hunter">
        <script type="application/json">
        [
            {"id": "a", "label": "Matrix a (top-left)", "min": -3, "max": 3, "val": 2, "step": 0.1},
            {"id": "b", "label": "Matrix b (top-right)", "min": -3, "max": 3, "val": 0, "step": 0.1},
            {"id": "c", "label": "Matrix c (bottom-left)", "min": -3, "max": 3, "val": 0, "step": 0.1},
            {"id": "d", "label": "Matrix d (bottom-right)", "min": -3, "max": 3, "val": 1, "step": 0.1},
            {"id": "reset", "label": "Reset (Diagonal)", "type": "button"},
            {"id": "symmetric", "label": "Symmetric Matrix", "type": "button"},
            {"id": "rotation", "label": "Pure Rotation", "type": "button"}
        ]
        </script>
</wasm-sim>

### Rotation Matrix

$$ \begin{bmatrix} 0&-1 \\ 1&0 \end{bmatrix} $$

This matrix rotates the grid $90\degree$ clock-wise

$$ \det{(A - \lambda I)} = \begin{vmatrix} -\lambda & -1 \\ 1 & -\lambda \end{vmatrix} $$

$$ \lambda^2 + 1 = 0 \implies \lambda = \pm i $$

Hence, there are **no real eigenvalues** and hence no real eigenvectors.

Thus, it is not possible to find the eigenvector in Pure Rotation preset above.

---

## Eigen Decomposition (Matrix Factorization)

Matrix $A$'s eigen decomposition is :

$$ A = PDP^{-1} $$

- $P$ : **Eigenvector Matrix**
    - Take all the eigenvectors ($v_1,v_2,\cdots$) and put them together as the columns of a single matrix

$$P = \begin{bmatrix} | & | \\ v_1 & v_2 \\ | & | \end{bmatrix}$$

- $D$ : **Eigenvalue Matrix**
    - This is a *Diagonal Matrix*.
        - All the off-diagonal values are 0.
    - Put all the corresponding eigenvalues ($\lambda_1, \lambda_2, \cdots $) on the main diagonal.

$$D = \begin{bmatrix} \lambda_1 & 0 \\ 0 & \lambda_2 \end{bmatrix}$$

- $P^{-1}$ : **Inverse**
    - Inverse of the eigenvector matrix

## Intuition of Eigen Decomposition

A matrix $A$ when applied to a vector may result in the the x-coordinate git mixed into the y-coordinate. Everything is coupled together. Eigenvectors are the *Axes of Rotation*. If we look at the world from the perspective of the eigenvectors, there is no mixing. There is only **stretching**.

> Eigen Decomposition breaks the transformation (matrix) $A$ into 3 distinct moves :

1. **$P^{-1}$ (The Twist)** :
    - Change the viewpoint. Rotate the entire coordinate system so that the eigenvectors become the new x and y axes.

2. **$D$ (The Stretch)** :
    - Now that we are aligned with eigenvectors, the transformation is just scaling along the axes. There is no more rotation or shearing.

3. **$P$ (The Untwist)** :
    - We rotate the coordinate system back to the original standard orientation.

>A dense matrix $A$ is like a diagonal matrix $D$ (which is computationally easier to use) that is "wearing a costume." The matrices $P$ and $P^{-1}$ are just the process of taking the costume off and putting it back on.

> Thus, it is also called **Diagonalization**, because the matrix $A$ is replaced with a diagonal matrix $D$.

## Why perform diagonalization ?

As established, $A = PDP^{-1}$.

$$ A^2 = (PDP^{-1})(PDP^{-1}) $$

$$ A^2 = PD(P^{-1}P)DP^{-1} = PD^2P^{-1} $$

Similarly, $ A^{100} = PD^{100}P^{-1} $

- $D^{100}$ is a lot easy to caluculate as it is a diagonal matrix. So, just take the power of the diagonal elements.

$$D = \begin{bmatrix} 2 & 0 \\ 0 & 3 \end{bmatrix} \implies D^{100} = \begin{bmatrix} 2^{100} & 0 \\ 0 & 3^{100} \end{bmatrix}$$

## Drawback

**Standard Eigenvectors and Eigenvalues are strictly for square matrices**.

$Av = \lambda v$. The Output Vector must be parallel to the Input Vector.

- For a Square matrix ($2\times2$) : 
    - Takes a 2D vector and outputs a 2D vector.
- For a Rectangular matrix ($3\times2$) :
    - Takes a 2D vector and outputs a 3D vector.
    - It changes the dimension, but the result should have been same as a scalar multiplication and that means dimensions must not change.

And as in most real world problems, data is not a square (eg. 1000 rows (users) & 50 columns (features)).

For this reason SVD is used for any shape.

---

## Coordinate Changer

This simulation visualizes the equation :

$$ A^t = PD^tP^{-1} $$

- **$A$** : transformation matrix in the standard (x, y) coordinates.
    - It is often coupled, i.e., x affects y, y affects x causing shearing and rotation.
- **$D$** : diagonal matrix containing the Eigenvalues ($\lambda_1, \lambda_2$). 
    - It represents pure stretching/shrinking with no rotation.

The eigen vectors are defined as :

$$ v_1 = \begin{bmatrix} \cos{(\theta)} \\ \sin{(\theta)} \end{bmatrix} $$
$$ v_2 = \begin{bmatrix} -\sin{(\theta)} \\ \cos{(\theta)} \end{bmatrix} $$

These 2 vectors are dynamically calculated using the slider **Eigenvector Angle**.
- These 2 vectors are always perpendicular, thus forming a pure rotation matrix $P$.

The matrix $A$ is not fixed in this simulation. For any point $v$, the transformation is calculated using $A=PDP^{-1}$.
- $P$ **(Basis Change)** : The matrix formed by the eigenvectors columns: $\begin{pmatrix} \cos\theta & -\sin\theta \\ \sin\theta & \cos\theta \end{pmatrix}$
- $D$ **(Diagonal Scaling)** : The matrix containing eigenvalues: $\begin{pmatrix} \lambda_1 & 0 \\ 0 & \lambda_2 \end{pmatrix}$

Thus to calculate $A^t$, (***On Left Side***) the sim computes :

$$A^t = P \cdot \begin{pmatrix} \lambda_1^t & 0 \\ 0 & \lambda_2^t \end{pmatrix} \cdot P^{-1}$$

- $P^{-1}$ : Takes the cross formed by the eigenvectors and rotate it to match the x,y axis shape.
- $D$ : Stretches $x$ by $\lambda_1^t$ and $y$ by $\lambda_2^t$.
- $P$ : Rotates the world back to original angle.

On ***Right Side*** :
- Let the Horizontal Axis be Eigenvector 1 ($v_1$).
- Let the Vertical Axis be Eigenvector 2 ($v_2$).

> So, because of the way the camera view is defined, the grid lines are the eigenvectors.

All the points on the smiley face undergo the respective transformation :
- Left : Calculates $A^t \vec{v}$ by doing the three-step dance ($P D^t P^{-1}$).
- Right : The transformation is by only Diagonal matrix $D^t$, because the camera is already "rotated" to align with the eigenvectors, we skip the rotation steps ($P$ and $P^{-1}$) entirely.
    - So the transformation becomes just the **scaling** : `v.x * lambda1` and `v.y * lambda2`.

<wasm-sim src="eigen_basis">
        <script type="application/json">
        [
            {"id": "t", "label": "Time (t)", "min": -2, "max": 4, "val": 0, "step": 0.05},
            {"id": "angle", "label": "Eigenvector Angle", "min": -3.14, "max": 3.14, "val": 0.52, "step": 0.1},
            {"id": "l1", "label": "Eigenvalue 1 (Stretch)", "min": 0.5, "max": 1.5, "val": 1.2, "step": 0.1},
            {"id": "l2", "label": "Eigenvalue 2 (Squish)", "min": 0.5, "max": 1.5, "val": 0.8, "step": 0.1},
            {"id": "reset", "label": "Reset", "type": "button"}
        ]
        </script>
</wasm-sim>

---

With this, the post on determinants and eigenvectors, their geometric implications and how diagonalization works and simplifies the task is complete.

<script src="static/js/wasm_engine.js"></script>